package admin

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	Addr        string
	EnablePprof bool
}

type Server struct {
	cfg Config
	srv *http.Server
}

func New(cfg Config) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	if cfg.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return &Server{
		cfg: cfg,
		srv: &http.Server{
			Addr:              cfg.Addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.cfg.Addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = s.srv.Shutdown(context.Background())
	}()
	return s.srv.Serve(ln)
}
