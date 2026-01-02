package tlsutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

const ALPN = "loom/1"

func ServerTLSConfig(nextProtos []string) (*tls.Config, error) {
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, err
	}
	if len(nextProtos) == 0 {
		nextProtos = []string{ALPN}
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   nextProtos,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func ClientTLSConfigInsecure(nextProtos []string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         nextProtos,
		MinVersion:         tls.VersionTLS13,
	}
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return tls.Certificate{}, err
	}
	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(365 * 24 * time.Hour)

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Loom"},
			CommonName:   "loom",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return tls.X509KeyPair(certPEM, keyPEM)
}
