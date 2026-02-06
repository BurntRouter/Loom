package router

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"log"
	"time"

	"github.com/BurntRouter/Loom/internal/auth"
	"github.com/BurntRouter/Loom/internal/config"
	"github.com/BurntRouter/Loom/internal/metrics"
	"github.com/BurntRouter/Loom/internal/protocol"
	quic "github.com/quic-go/quic-go"
)

type Server struct {
	Addr string
	TLS  *tls.Config
	QUIC *quic.Config

	Rooms *RoomManager
	Auth  *AuthContext
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	tlsConf := s.TLS
	if tlsConf == nil {
		return errors.New("router: TLS config is required")
	}

	quicConf := s.QUIC
	if quicConf == nil {
		quicConf = &quic.Config{}
	}
	if quicConf.KeepAlivePeriod == 0 {
		quicConf.KeepAlivePeriod = 15 * time.Second
	}
	listener, err := quic.ListenAddr(s.Addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("loom: listening on %s", s.Addr)
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		metrics.Connections.WithLabelValues("quic").Inc()
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn *quic.Conn) {
	defer metrics.Connections.WithLabelValues("quic").Dec()
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleStream(ctx, conn, stream)
	}
}

type quicBidiStream struct {
	r *bufio.Reader
	s *quic.Stream
}

func (q *quicBidiStream) Read(p []byte) (int, error)  { return q.r.Read(p) }
func (q *quicBidiStream) Write(p []byte) (int, error) { return q.s.Write(p) }
func (q *quicBidiStream) Close() error                { return q.s.Close() }
func (q *quicBidiStream) Context() context.Context    { return q.s.Context() }

func (s *Server) handleStream(ctx context.Context, conn *quic.Conn, stream *quic.Stream) {
	br := bufio.NewReader(stream)
	role, name, room, token, err := protocol.ReadHello(br, s.Rooms.cfg.MaxNameBytes, s.Rooms.cfg.MaxRoomBytes, s.Rooms.cfg.MaxTokenBytes)
	if err != nil {
		_ = stream.Close()
		return
	}

	var roleEnum auth.Role
	switch role {
	case protocol.RoleProducer:
		roleEnum = auth.RoleProduce
	case protocol.RoleConsumer:
		roleEnum = auth.RoleConsume
	default:
		_ = stream.Close()
		return
	}

	authCtx := s.Auth
	if authCtx == nil {
		authCtx = &AuthContext{Mode: config.AuthModeDisabled}
	}
	d := authCtx.authorizeQUIC(conn, token, room, roleEnum)
	if !d.Allowed {
		_ = stream.Close()
		return
	}

	roleLabel := ""
	switch role {
	case protocol.RoleProducer:
		roleLabel = "producer"
	case protocol.RoleConsumer:
		roleLabel = "consumer"
	}
	metrics.Streams.WithLabelValues("quic", roleLabel, room).Inc()
	defer metrics.Streams.WithLabelValues("quic", roleLabel, room).Dec()

	r := s.Rooms.Get(room)
	switch role {
	case protocol.RoleConsumer:
		cs := &quicBidiStream{r: br, s: stream}
		id, err := r.RegisterConsumer(name, cs)
		if err != nil {
			_ = stream.Close()
			return
		}
		log.Printf("loom: consumer connected room=%q id=%s name=%q", room, id, name)
		select {
		case <-ctx.Done():
		case <-stream.Context().Done():
		}
	case protocol.RoleProducer:
		// Check if this producer is blocked due to repeated errors
		if s.Rooms.errorTracker != nil {
			producerKey := room + ":" + name + ":" + conn.RemoteAddr().String()
			if s.Rooms.errorTracker.IsBlocked(producerKey) {
				log.Printf("loom: rejecting blocked producer room=%q name=%q addr=%s", room, name, conn.RemoteAddr())
				metrics.BlockedProducers.WithLabelValues(room).Inc()
				_ = stream.Close()
				return
			}
		}

		if err := r.HandleProducer(ctx, br); err != nil {
			if !errors.Is(err, context.Canceled) {
				remoteAddr := conn.RemoteAddr().String()
				producerKey := room + ":" + name + ":" + remoteAddr

				// Check if this is a protocol error (likely client bug)
				errMsg := err.Error()
				isProtocolError := errors.Is(err, protocol.ErrBadHandshake) ||
					errMsg == "protocol: empty key - possible stream corruption" ||
					(len(errMsg) > 9 && errMsg[:9] == "protocol:")

				if isProtocolError {
					// Categorize error type for metrics
					errorType := "unknown"
					if errMsg == "protocol: empty key - possible stream corruption" {
						errorType = "empty_key"
					} else if len(errMsg) > 20 && errMsg[:20] == "protocol: chunk too" {
						errorType = "chunk_too_large"
					} else if len(errMsg) > 19 && errMsg[:19] == "protocol: key too" {
						errorType = "key_too_large"
					} else if len(errMsg) > 25 && errMsg[:25] == "protocol: invalid varint" {
						errorType = "invalid_varint"
					}
					metrics.ProtocolErrors.WithLabelValues(room, errorType).Inc()

					if s.Rooms.errorTracker != nil {
						if blocked := s.Rooms.errorTracker.RecordError(producerKey); blocked {
							metrics.BlockedProducers.WithLabelValues(room).Inc()
							log.Printf("loom: BLOCKED producer due to repeated protocol errors room=%q name=%q addr=%s", room, name, remoteAddr)
						}
					}
				}

				log.Printf("loom: producer stream error room=%q name=%q addr=%s: %v", room, name, remoteAddr, err)
			}
		}
		_ = stream.Close()
	default:
		_ = stream.Close()
	}
}
