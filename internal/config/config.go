package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Transport string

type PartitionFullBehavior string

type ChunkFullBehavior string

const (
	TransportQUIC Transport = "quic"
	TransportH3   Transport = "h3"

	PartitionFullDropNewest PartitionFullBehavior = "drop_newest"
	PartitionFullDropOldest PartitionFullBehavior = "drop_oldest"
	PartitionFullBlock      PartitionFullBehavior = "block"

	// ChunkFullDrop drops the whole message when the per-message chunk queue is full.
	ChunkFullDrop  ChunkFullBehavior = "drop"
	ChunkFullBlock ChunkFullBehavior = "block"
)

type Config struct {
	Transport Transport `yaml:"transport"`

	Server ServerConfig `yaml:"server"`
	Admin  AdminConfig  `yaml:"admin"`
	Auth   AuthConfig   `yaml:"auth"`
	Router RouterConfig `yaml:"router"`
}

type ServerConfig struct {
	Addr string    `yaml:"addr"`
	TLS  TLSConfig `yaml:"tls"`
}

type RouterConfig struct {
	PartitionCount int `yaml:"partition_count"`

	MaxNameBytes  int `yaml:"max_name_bytes"`
	MaxRoomBytes  int `yaml:"max_room_bytes"`
	MaxTokenBytes int `yaml:"max_token_bytes"`
	MaxKeyBytes   int `yaml:"max_key_bytes"`

	MaxChunkBytes int    `yaml:"max_chunk_bytes"`
	MaxMessageB   uint64 `yaml:"max_message_bytes"`

	ConsumerQueueDepth int `yaml:"max_backlog_depth"`

	PartitionFullBehavior PartitionFullBehavior `yaml:"partition_full_behavior"`
	ChunkFullBehavior     ChunkFullBehavior     `yaml:"chunk_full_behavior"`
}

func Default() Config {
	return Config{
		Transport: TransportQUIC,
		Server: ServerConfig{
			Addr: ":4242",
			TLS: TLSConfig{
				InsecureSkipVerify: true,
			},
		},
		Admin: AdminConfig{
			Addr:        ":9090",
			EnablePprof: false,
		},
		Auth: AuthConfig{Mode: AuthModeDisabled},

		Router: RouterConfig{
			PartitionCount:        64,
			MaxNameBytes:          128,
			MaxRoomBytes:          128,
			MaxTokenBytes:         1024,
			MaxKeyBytes:           256,
			MaxChunkBytes:         64 << 10,
			MaxMessageB:           256 << 20,
			ConsumerQueueDepth:    128,
			PartitionFullBehavior: PartitionFullDropNewest,
			ChunkFullBehavior:     ChunkFullDrop,
		},
	}
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Transport != TransportQUIC && c.Transport != TransportH3 {
		return fmt.Errorf("config: unknown transport %q", c.Transport)
	}
	if c.Router.MaxNameBytes <= 0 {
		return errors.New("config: router.max_name_bytes must be > 0")
	}
	if c.Router.MaxRoomBytes <= 0 {
		return errors.New("config: router.max_room_bytes must be > 0")
	}
	if c.Router.MaxTokenBytes <= 0 {
		return errors.New("config: router.max_token_bytes must be > 0")
	}
	if c.Router.MaxChunkBytes <= 0 {
		return errors.New("config: router.max_chunk_bytes must be > 0")
	}
	if c.Router.MaxMessageB == 0 {
		return errors.New("config: router.max_message_bytes must be > 0")
	}
	if c.Router.ConsumerQueueDepth <= 0 {
		return errors.New("config: router.max_backlog_depth must be > 0")
	}
	if c.Router.PartitionCount <= 0 {
		return errors.New("config: router.partition_count must be > 0")
	}
	// Backward compatibility: "drop" means drop_newest.
	if c.Router.PartitionFullBehavior == "drop" {
		c.Router.PartitionFullBehavior = PartitionFullDropNewest
	}
	if c.Router.PartitionFullBehavior != PartitionFullDropNewest && c.Router.PartitionFullBehavior != PartitionFullDropOldest && c.Router.PartitionFullBehavior != PartitionFullBlock {
		return fmt.Errorf("config: unknown router.partition_full_behavior %q", c.Router.PartitionFullBehavior)
	}
	if c.Router.ChunkFullBehavior != ChunkFullDrop && c.Router.ChunkFullBehavior != ChunkFullBlock {
		return fmt.Errorf("config: unknown router.chunk_full_behavior %q", c.Router.ChunkFullBehavior)
	}

	if c.Server.Addr == "" {
		return errors.New("config: server.addr is required")
	}
	if c.Server.TLS.CertFile != "" && c.Server.TLS.KeyFile == "" {
		return errors.New("config: server.tls.key_file is required when cert_file is set")
	}
	if c.Server.TLS.KeyFile != "" && c.Server.TLS.CertFile == "" {
		return errors.New("config: server.tls.cert_file is required when key_file is set")
	}
	if (c.Auth.Mode == AuthModeMTLS || c.Auth.Mode == AuthModeBoth || c.Server.TLS.RequireClientCert) && c.Server.TLS.ClientCAFile == "" {
		return errors.New("config: server.tls.client_ca_file is required for mTLS")
	}
	if c.Auth.Mode == AuthModeBoth {
		c.Server.TLS.RequireClientCert = true
	}

	return nil
}
