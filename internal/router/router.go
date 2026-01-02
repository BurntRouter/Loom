package router

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"github.com/BurntRouter/Loom/internal/hash"
	"github.com/BurntRouter/Loom/internal/protocol"
)

type Config struct {
	PartitionCount int

	MaxNameBytes    int
	MaxRoomBytes    int
	MaxTokenBytes   int
	MaxKeyBytes     int
	MaxChunkBytes   int
	MaxMessageBytes uint64

	ConsumerQueueDepth int
	MessageChunkQueue  int

	PartitionFullBehavior string
	ChunkFullBehavior     string
}

const (
	PartitionFullDropNewest = "drop_newest"
	PartitionFullDropOldest = "drop_oldest"
	PartitionFullBlock      = "block"

	ChunkFullDrop  = "drop" // drops the whole message on chunk queue pressure
	ChunkFullBlock = "block"
)

func defaultMessageChunkQueue(maxChunkBytes int) int {
	if maxChunkBytes <= 0 {
		return 1
	}
	const targetBufferedBytes = 1 << 20 // 1MiB buffered per in-flight message
	q := targetBufferedBytes / maxChunkBytes
	if q < 1 {
		q = 1
	}
	return q
}

func DefaultConfig() Config {
	cfg := Config{
		PartitionCount:        64,
		MaxNameBytes:          128,
		MaxRoomBytes:          128,
		MaxTokenBytes:         1024,
		MaxKeyBytes:           256,
		MaxChunkBytes:         64 << 10,
		MaxMessageBytes:       256 << 20,
		ConsumerQueueDepth:    128,
		PartitionFullBehavior: PartitionFullDropNewest,
		ChunkFullBehavior:     ChunkFullDrop,
	}
	cfg.MessageChunkQueue = defaultMessageChunkQueue(cfg.MaxChunkBytes)
	return cfg
}

type Router struct {
	cfg Config
	rh  *hash.Rendezvous

	partSeed maphash.Seed

	mu        sync.RWMutex
	consumers map[string]*consumerState
	seq       atomic.Uint64
	msgSeq    atomic.Uint64
}

func New(cfg Config) *Router {
	return &Router{
		cfg:       cfg,
		rh:        hash.NewRendezvous(),
		partSeed:  maphash.MakeSeed(),
		consumers: make(map[string]*consumerState),
	}
}

func (r *Router) SetConfig(cfg Config) {
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
}

type consumerState struct {
	id     string
	name   string
	stream Stream
	send   chan *routedMessage
	done   chan struct{}
	active atomic.Bool

	pmu     sync.Mutex
	pending map[uint64]*routedMessage
}

type routedMessage struct {
	key          []byte
	declaredSize uint64
	msgID        uint64
	chunks       chan []byte
	canceled     atomic.Bool

	ackedOK atomic.Bool
	acked   chan struct{}
	once    sync.Once
}

func (m *routedMessage) markAcked(ok bool) {
	if ok {
		m.ackedOK.Store(true)
	}
	m.once.Do(func() { close(m.acked) })
}

func (r *Router) RegisterConsumer(name string, stream Stream) (string, error) {
	id := fmt.Sprintf("c-%d", r.seq.Add(1))
	c := &consumerState{
		id:      id,
		name:    name,
		stream:  stream,
		send:    make(chan *routedMessage, r.cfg.ConsumerQueueDepth),
		done:    make(chan struct{}),
		pending: make(map[uint64]*routedMessage),
	}
	c.active.Store(true)

	r.mu.Lock()
	r.consumers[id] = c
	r.mu.Unlock()

	go r.runConsumerReader(c)
	go r.runConsumerWriter(c)
	return id, nil
}

func (r *Router) removeConsumer(id string) {
	r.mu.Lock()
	delete(r.consumers, id)
	r.mu.Unlock()
}

func (r *Router) runConsumerWriter(c *consumerState) {
	defer func() {
		c.pmu.Lock()
		for _, m := range c.pending {
			m.markAcked(false)
		}
		c.pmu.Unlock()

		c.active.Store(false)
		close(c.done)
		r.removeConsumer(c.id)
		_ = c.stream.Close()
	}()

	w := bufio.NewWriter(c.stream)
	for {
		select {
		case <-c.stream.Context().Done():
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			if !c.active.Load() {
				msg.canceled.Store(true)
				msg.markAcked(false)
				continue
			}

			c.pmu.Lock()
			c.pending[msg.msgID] = msg
			c.pmu.Unlock()

			if err := protocol.WriteMessageHeader(w, msg.key, msg.declaredSize, msg.msgID); err != nil {
				log.Printf("consumer %s write header: %v", c.id, err)
				return
			}
			for chunk := range msg.chunks {
				if err := protocol.WriteChunk(w, chunk); err != nil {
					log.Printf("consumer %s write chunk: %v", c.id, err)
					return
				}
			}
			if err := protocol.WriteEndOfMessage(w); err != nil {
				log.Printf("consumer %s write eom: %v", c.id, err)
				return
			}
			if err := w.Flush(); err != nil {
				log.Printf("consumer %s flush: %v", c.id, err)
				return
			}

			select {
			case <-msg.acked:
			case <-c.stream.Context().Done():
				return
			}

			c.pmu.Lock()
			delete(c.pending, msg.msgID)
			c.pmu.Unlock()
		}
	}
}

func (r *Router) runConsumerReader(c *consumerState) {
	br := bufio.NewReader(c.stream)
	for {
		ft, msgID, err := protocol.ReadFrame(br)
		if err != nil {
			return
		}
		if ft != protocol.FrameAck {
			continue
		}
		c.pmu.Lock()
		m := c.pending[msgID]
		c.pmu.Unlock()
		if m != nil {
			m.markAcked(true)
		}
	}
}

func (r *Router) HandleProducer(ctx context.Context, br *bufio.Reader) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, err := protocol.ReadMessageHeader(br, r.cfg.MaxKeyBytes)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if hdr.DeclaredSize > 0 && uint64(hdr.DeclaredSize) > r.cfg.MaxMessageBytes {
			if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
				return err
			}
			continue
		}

		c := r.pickConsumer(hdr.Key)
		if c == nil {
			if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
				return err
			}
			continue
		}
		consumerDone := c.done
		select {
		case <-consumerDone:
			if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
				return err
			}
			continue
		default:
		}

		msg := &routedMessage{
			key:          hdr.Key,
			declaredSize: hdr.DeclaredSize,
			msgID:        r.msgSeq.Add(1),
			chunks:       make(chan []byte, r.cfg.MessageChunkQueue),
			acked:        make(chan struct{}),
		}

		switch r.cfg.PartitionFullBehavior {
		case PartitionFullBlock:
			select {
			case c.send <- msg:
			case <-consumerDone:
				if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
					return err
				}
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		case PartitionFullDropOldest:
			select {
			case c.send <- msg:
				// queued
			default:
				select {
				case dropped := <-c.send:
					dropped.canceled.Store(true)
				default:
				}
				select {
				case c.send <- msg:
				default:
					if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
						return err
					}
					continue
				}
			}
		default: // drop newest
			select {
			case c.send <- msg:
			default:
				if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
					return err
				}
				continue
			}
		}

		dropped := false
		var total uint64
	readChunks:
		for {
			chunk, done, err := protocol.ReadChunk(br, r.cfg.MaxChunkBytes)
			if err != nil {
				close(msg.chunks)
				return err
			}
			if done {
				close(msg.chunks)
				select {
				case <-msg.acked:
				case <-consumerDone:
					msg.markAcked(false)
				case <-ctx.Done():
					msg.markAcked(false)
					return ctx.Err()
				}
				break
			}

			total += uint64(len(chunk))
			if total > r.cfg.MaxMessageBytes {
				dropped = true
				close(msg.chunks)
				if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
					return err
				}
				break readChunks
			}

			if msg.canceled.Load() {
				dropped = true
			}
			if dropped {
				continue
			}

			switch r.cfg.ChunkFullBehavior {
			case ChunkFullBlock:
				select {
				case msg.chunks <- chunk:
				case <-consumerDone:
					dropped = true
					close(msg.chunks)
					if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
						return err
					}
					break readChunks
				case <-ctx.Done():
					close(msg.chunks)
					return ctx.Err()
				}
			default:
				select {
				case <-consumerDone:
					dropped = true
					close(msg.chunks)
					if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
						return err
					}
					break readChunks
				default:
				}
				select {
				case msg.chunks <- chunk:
				default:
					dropped = true
					close(msg.chunks)
					if err := protocol.DiscardMessage(br, r.cfg.MaxChunkBytes); err != nil {
						return err
					}
					break readChunks
				}
			}
		}
	}
}

func (r *Router) pickConsumer(key []byte) *consumerState {
	r.mu.RLock()
	ids := make([]string, 0, len(r.consumers))
	for id, c := range r.consumers {
		if c.active.Load() {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()

	partKey := r.partitionKey(key)
	id, ok := r.rh.Pick(partKey, ids)
	if !ok {
		return nil
	}
	r.mu.RLock()
	c := r.consumers[id]
	r.mu.RUnlock()
	if c == nil || !c.active.Load() {
		return nil
	}
	return c
}

func (r *Router) partitionKey(key []byte) []byte {
	var h maphash.Hash
	h.SetSeed(r.partSeed)
	_, _ = h.Write(key)
	part := h.Sum64() % uint64(r.cfg.PartitionCount)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, part)
	return buf
}
