package router

import (
	"crypto/x509"

	"github.com/BurntRouter/Loom/internal/auth"
	"github.com/BurntRouter/Loom/internal/config"
	quic "github.com/quic-go/quic-go"
)

type AuthContext struct {
	Mode       config.AuthMode
	Authorizer *auth.Authorizer
}

func (a AuthContext) authorizeQUIC(conn *quic.Conn, token string, room string, role auth.Role) auth.Decision {
	switch a.Mode {
	case config.AuthModeDisabled:
		return auth.Decision{Allowed: true, Principal: "anonymous"}
	case config.AuthModeToken:
		return a.Authorizer.AuthorizeToken(token, room, role)
	case config.AuthModeMTLS:
		return a.Authorizer.AuthorizePeerCert(peerCertFromQUIC(conn), room, role)
	case config.AuthModeBoth:
		d1 := a.Authorizer.AuthorizeToken(token, room, role)
		if !d1.Allowed {
			return d1
		}
		d2 := a.Authorizer.AuthorizePeerCert(peerCertFromQUIC(conn), room, role)
		if !d2.Allowed {
			d2.Principal = d1.Principal
			return d2
		}
		return d1
	default:
		return auth.Decision{Allowed: false, Reason: "unknown auth mode"}
	}
}

func peerCertFromQUIC(conn *quic.Conn) *x509.Certificate {
	st := conn.ConnectionState()
	if len(st.TLS.PeerCertificates) == 0 {
		return nil
	}
	return st.TLS.PeerCertificates[0]
}
