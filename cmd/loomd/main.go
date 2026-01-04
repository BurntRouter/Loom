package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/BurntRouter/Loom/internal/admin"
	"github.com/BurntRouter/Loom/internal/auth"
	"github.com/BurntRouter/Loom/internal/config"
	"github.com/BurntRouter/Loom/internal/metrics"
	"github.com/BurntRouter/Loom/internal/router"
	"github.com/BurntRouter/Loom/internal/tlsutil"
	"github.com/quic-go/quic-go/http3"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfgPath := os.Getenv("LOOM_CONFIG")
	if cfgPath == "" {
		cfgPath = "loom.yaml"
	}
	if len(os.Args) >= 2 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	runServe(cfg, cfgPath)
}

func runServe(cfg config.Config, cfgPath string) {
	metrics.Register()

	buildRouterCfg := func(c config.Config) router.Config {
		messageChunkQueue := 1
		if c.Router.MaxChunkBytes > 0 {
			const targetBufferedBytes = 1 << 20 // 1MiB buffered per in-flight message
			messageChunkQueue = targetBufferedBytes / c.Router.MaxChunkBytes
			if messageChunkQueue < 1 {
				messageChunkQueue = 1
			}
		}
		return router.Config{
			PartitionCount:        c.Router.PartitionCount,
			MaxNameBytes:          c.Router.MaxNameBytes,
			MaxRoomBytes:          c.Router.MaxRoomBytes,
			MaxTokenBytes:         c.Router.MaxTokenBytes,
			MaxKeyBytes:           c.Router.MaxKeyBytes,
			MaxChunkBytes:         c.Router.MaxChunkBytes,
			MaxMessageBytes:       c.Router.MaxMessageB,
			ConsumerQueueDepth:    c.Router.ConsumerQueueDepth,
			MessageChunkQueue:     messageChunkQueue,
			PartitionFullBehavior: string(c.Router.PartitionFullBehavior),
			ChunkFullBehavior:     string(c.Router.ChunkFullBehavior),
			QueueType:             router.QueueTypePartitioned,
		}
	}

	buildRoomCfg := func(c config.Config, base router.Config) map[string]router.Config {
		m := make(map[string]router.Config)
		for _, rc := range c.Rooms {
			rc2 := base
			if rc.MaxDepth > 0 {
				rc2.ConsumerQueueDepth = rc.MaxDepth
			}
			if rc.PartitionFullBehavior != "" {
				rc2.PartitionFullBehavior = string(rc.PartitionFullBehavior)
			}
			if rc.QueueType != "" {
				rc2.QueueType = string(rc.QueueType)
			}
			m[rc.Name] = rc2
		}
		return m
	}

	rCfg := buildRouterCfg(cfg)
	rooms := router.NewRoomManager(rCfg, buildRoomCfg(cfg, rCfg))

	authz := auth.FromConfig(cfg.Auth)
	authCtx := &router.AuthContext{Mode: cfg.Auth.Mode, Authorizer: authz}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adminSrv := admin.New(admin.Config{Addr: cfg.Admin.Addr, EnablePprof: cfg.Admin.EnablePprof})
	go func() {
		if err := adminSrv.ListenAndServe(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("admin server error: %v", err)
			cancel()
		}
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				next, err := config.Load(cfgPath)
				if err != nil {
					log.Printf("reload failed: %v", err)
					continue
				}
				rc := buildRouterCfg(next)
				rooms.UpdateConfig(rc, buildRoomCfg(next, rc))
				authCtx.Mode = next.Auth.Mode
				authCtx.Authorizer = auth.FromConfig(next.Auth)
				log.Printf("reloaded config: %s", cfgPath)
			default:
				cancel()
				return
			}
		}
	}()

	nextProtos := []string{tlsutil.ALPN}
	if cfg.Transport == config.TransportH3 {
		nextProtos = []string{http3.NextProtoH3}
	}
	serverTLS, err := serverTLSConfig(cfg.Server.TLS, nextProtos)
	if err != nil {
		log.Fatal(err)
	}

	switch cfg.Transport {
	case config.TransportH3:
		s := &router.H3Server{Addr: cfg.Server.Addr, TLS: serverTLS, Rooms: rooms, Auth: authCtx}
		if err := s.ListenAndServe(ctx); err != nil {
			log.Fatal(err)
		}
	default:
		s := &router.Server{Addr: cfg.Server.Addr, TLS: serverTLS, Rooms: rooms, Auth: authCtx}
		if err := s.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}
}
