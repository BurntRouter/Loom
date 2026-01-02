package router

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"log"

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
		if err := r.HandleProducer(ctx, br); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("loom: producer stream error room=%q: %v", room, err)
			}
		}
		_ = stream.Close()
	default:
		_ = stream.Close()
	}
}
