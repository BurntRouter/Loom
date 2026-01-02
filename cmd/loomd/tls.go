package main

import (
	"crypto/tls"

	"github.com/BurntRouter/Loom/internal/config"
	"github.com/BurntRouter/Loom/internal/tlsutil"
)

func serverTLSConfig(c config.TLSConfig, nextProtos []string) (*tls.Config, error) {
	if c.CertFile != "" || c.KeyFile != "" {
		return tlsutil.ServerTLSConfigFromFiles(nextProtos, tlsutil.ServerTLSFiles{
			CertFile:          c.CertFile,
			KeyFile:           c.KeyFile,
			ClientCAFile:      c.ClientCAFile,
			RequireClientCert: c.RequireClientCert,
		})
	}
	if c.RequireClientCert {
		return nil, tlsutil.ErrClientCertNotSupported
	}
	return tlsutil.ServerTLSConfig(nextProtos)
}

func clientTLSConfig(c config.TLSConfig, nextProtos []string) (*tls.Config, error) {
	if c.InsecureSkipVerify {
		cfg := tlsutil.ClientTLSConfigInsecure(nextProtos)
		if c.ServerName != "" {
			cfg.ServerName = c.ServerName
		}
		return cfg, nil
	}
	cfg, err := tlsutil.ClientTLSConfigFromFiles(nextProtos, tlsutil.ClientTLSFiles{
		CAFile:     c.CAFile,
		CertFile:   c.CertFile,
		KeyFile:    c.KeyFile,
		ServerName: c.ServerName,
	})
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
