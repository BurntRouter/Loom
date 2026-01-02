package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var ErrClientCertNotSupported = errors.New("tlsutil: require_client_cert needs file-based server cert")

type ServerTLSFiles struct {
	CertFile          string
	KeyFile           string
	ClientCAFile      string
	RequireClientCert bool
}

type ClientTLSFiles struct {
	CAFile     string
	CertFile   string
	KeyFile    string
	ServerName string
}

func ServerTLSConfigFromFiles(nextProtos []string, files ServerTLSFiles) (*tls.Config, error) {
	if len(nextProtos) == 0 {
		nextProtos = []string{ALPN}
	}
	cert, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   nextProtos,
		MinVersion:   tls.VersionTLS13,
	}
	if files.RequireClientCert {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		pool := x509.NewCertPool()
		b, err := os.ReadFile(files.ClientCAFile)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("tlsutil: failed to parse client CA")
		}
		cfg.ClientCAs = pool
	}
	return cfg, nil
}

func ClientTLSConfigFromFiles(nextProtos []string, files ClientTLSFiles) (*tls.Config, error) {
	if len(nextProtos) == 0 {
		nextProtos = []string{ALPN}
	}
	cfg := &tls.Config{NextProtos: nextProtos, MinVersion: tls.VersionTLS13}
	if files.ServerName != "" {
		cfg.ServerName = files.ServerName
	}
	if files.CAFile != "" {
		pool := x509.NewCertPool()
		b, err := os.ReadFile(files.CAFile)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("tlsutil: failed to parse server CA")
		}
		cfg.RootCAs = pool
	}
	if files.CertFile != "" || files.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
