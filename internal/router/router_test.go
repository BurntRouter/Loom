package router

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/BurntRouter/Loom/internal/protocol"
)

type ctxConn struct {
	net.Conn
	ctx context.Context
}

func (c *ctxConn) Context() context.Context { return c.ctx }

func TestProducerWaitsForConsumerAck(t *testing.T) {
	r := New(DefaultConfig())

	c1, c2 := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerStream := &ctxConn{Conn: c1, ctx: ctx}
	clientSide := &ctxConn{Conn: c2, ctx: ctx}

	_, err := r.RegisterConsumer("c", consumerStream)
	if err != nil {
		t.Fatal(err)
	}

	// Build one message from a producer.
	var prod bytes.Buffer
	pw := bufio.NewWriter(&prod)
	if err := protocol.WriteMessageHeader(pw, []byte("key"), 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteChunk(pw, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteEndOfMessage(pw); err != nil {
		t.Fatal(err)
	}
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}

	// Start producer handling in background; it should block until ACK.
	pctx, pcancel := context.WithCancel(context.Background())
	defer pcancel()

	var wg sync.WaitGroup
	wg.Add(1)
	prodDone := make(chan error, 1)
	go func() {
		defer wg.Done()
		prodDone <- r.HandleProducer(pctx, bufio.NewReader(bytes.NewReader(prod.Bytes())))
	}()

	// Read the routed message from consumer side.
	cr := bufio.NewReader(clientSide)
	hdr, err := protocol.ReadMessageHeader(cr, 256)
	if err != nil {
		t.Fatal(err)
	}
	if string(hdr.Key) != "key" {
		t.Fatalf("unexpected key %q", string(hdr.Key))
	}
	if hdr.MsgID == 0 {
		t.Fatal("expected server-assigned msg_id")
	}

	chunk, done, err := protocol.ReadChunk(cr, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if done || string(chunk) != "hello" {
		t.Fatalf("unexpected chunk: done=%v chunk=%q", done, string(chunk))
	}
	_, done, err = protocol.ReadChunk(cr, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected eom")
	}

	select {
	case <-prodDone:
		t.Fatal("producer returned before ACK")
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	// ACK and ensure producer completes.
	cw := bufio.NewWriter(clientSide)
	if err := protocol.WriteAck(cw, hdr.MsgID); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-prodDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for producer completion")
	}

	wg.Wait()
	_ = clientSide.Close()
}

func TestFanoutDeliversToAllConsumers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.QueueType = QueueTypeFanout
	r := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c1s, c1c := net.Pipe()
	c2s, c2c := net.Pipe()
	defer c1c.Close()
	defer c2c.Close()

	_, err := r.RegisterConsumer("c1", &ctxConn{Conn: c1s, ctx: ctx})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RegisterConsumer("c2", &ctxConn{Conn: c2s, ctx: ctx})
	if err != nil {
		t.Fatal(err)
	}

	var prod bytes.Buffer
	pw := bufio.NewWriter(&prod)
	if err := protocol.WriteMessageHeader(pw, []byte("key"), 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteChunk(pw, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteEndOfMessage(pw); err != nil {
		t.Fatal(err)
	}
	if err := pw.Flush(); err != nil {
		t.Fatal(err)
	}

	pctx, pcancel := context.WithCancel(context.Background())
	defer pcancel()

	prodDone := make(chan error, 1)
	go func() {
		prodDone <- r.HandleProducer(pctx, bufio.NewReader(bytes.NewReader(prod.Bytes())))
	}()

	readOne := func(conn net.Conn) uint64 {
		cr := bufio.NewReader(conn)
		hdr, err := protocol.ReadMessageHeader(cr, 256)
		if err != nil {
			t.Fatal(err)
		}
		if string(hdr.Key) != "key" {
			t.Fatalf("unexpected key %q", string(hdr.Key))
		}
		chunk, done, err := protocol.ReadChunk(cr, 64<<10)
		if err != nil {
			t.Fatal(err)
		}
		if done || string(chunk) != "hello" {
			t.Fatalf("unexpected chunk: done=%v chunk=%q", done, string(chunk))
		}
		_, done, err = protocol.ReadChunk(cr, 64<<10)
		if err != nil {
			t.Fatal(err)
		}
		if !done {
			t.Fatal("expected eom")
		}
		return hdr.MsgID
	}

	id1 := readOne(c1c)
	id2 := readOne(c2c)
	if id1 == 0 || id2 == 0 {
		t.Fatal("expected server-assigned msg_id")
	}

	// Producer should wait for both ACKs.
	select {
	case <-prodDone:
		t.Fatal("producer returned before ACKs")
	case <-time.After(50 * time.Millisecond):
	}

	cw1 := bufio.NewWriter(c1c)
	if err := protocol.WriteAck(cw1, id1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-prodDone:
		t.Fatal("producer returned before second ACK")
	case <-time.After(50 * time.Millisecond):
	}

	cw2 := bufio.NewWriter(c2c)
	if err := protocol.WriteAck(cw2, id2); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-prodDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for producer completion")
	}
}
