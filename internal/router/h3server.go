package router

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/BurntRouter/Loom/internal/auth"
	"github.com/BurntRouter/Loom/internal/config"
	"github.com/BurntRouter/Loom/internal/metrics"
	"github.com/BurntRouter/Loom/internal/protocol"
	"github.com/quic-go/quic-go/http3"
)

type H3Server struct {
	Addr string
	TLS  *tls.Config

	Rooms *RoomManager
	Auth  *AuthContext
}

func (s *H3Server) ListenAndServe(ctx context.Context) error {
	tlsConf := s.TLS
	if tlsConf == nil {
		return errors.New("router: TLS config is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		br := bufio.NewReader(r.Body)
		role, name, room, token, err := protocol.ReadHello(br, s.Rooms.cfg.MaxNameBytes, s.Rooms.cfg.MaxRoomBytes, s.Rooms.cfg.MaxTokenBytes)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var roleEnum auth.Role
		var roleLabel string
		switch role {
		case protocol.RoleProducer:
			roleEnum = auth.RoleProduce
			roleLabel = "producer"
		case protocol.RoleConsumer:
			roleEnum = auth.RoleConsume
			roleLabel = "consumer"
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		authCtx := s.Auth
		if authCtx == nil {
			authCtx = &AuthContext{Mode: config.AuthModeDisabled}
		}
		d := authCtx.authorizeHTTP(r, token, room, roleEnum)
		if !d.Allowed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		metrics.Streams.WithLabelValues("h3", roleLabel, room).Inc()
		defer metrics.Streams.WithLabelValues("h3", roleLabel, room).Dec()

		roomRouter := s.Rooms.Get(room)
		switch role {
		case protocol.RoleProducer:
			if err := roomRouter.HandleProducer(r.Context(), br); err != nil {
				log.Printf("loom: h3 producer error room=%q: %v", room, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		case protocol.RoleConsumer:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			ws := &httpBidiStream{r: br, body: r.Body, w: w, ctx: r.Context()}
			id, err := roomRouter.RegisterConsumer(name, ws)
			if err != nil {
				return
			}
			log.Printf("loom: h3 consumer connected room=%q id=%s name=%q", room, id, name)
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	srv := &http3.Server{
		Addr:      s.Addr,
		Handler:   mux,
		TLSConfig: tlsConf,
	}
	defer srv.Close()

	log.Printf("loom: http3 listening on %s", s.Addr)
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return srv.ListenAndServe()
}

type httpBidiStream struct {
	r    *bufio.Reader
	body io.ReadCloser
	w    http.ResponseWriter
	ctx  context.Context
}

func (h *httpBidiStream) Read(p []byte) (int, error) { return h.r.Read(p) }
func (h *httpBidiStream) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	if f, ok := h.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}
func (h *httpBidiStream) Close() error             { return h.body.Close() }
func (h *httpBidiStream) Context() context.Context { return h.ctx }

type httpResponseWriteStream struct {
	w   http.ResponseWriter
	ctx context.Context
}

func (h *httpResponseWriteStream) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	if f, ok := h.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

func (h *httpResponseWriteStream) Close() error             { return nil }
func (h *httpResponseWriteStream) Context() context.Context { return h.ctx }
