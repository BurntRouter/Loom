package router

import (
	"crypto/x509"
	"net/http"

	"github.com/BurntRouter/Loom/internal/auth"
	"github.com/BurntRouter/Loom/internal/config"
)

func (a AuthContext) authorizeHTTP(r *http.Request, token string, room string, role auth.Role) auth.Decision {
	switch a.Mode {
	case config.AuthModeDisabled:
		return auth.Decision{Allowed: true, Principal: "anonymous"}
	case config.AuthModeToken:
		return a.Authorizer.AuthorizeToken(token, room, role)
	case config.AuthModeMTLS:
		return a.Authorizer.AuthorizePeerCert(peerCertFromHTTP(r), room, role)
	case config.AuthModeBoth:
		d1 := a.Authorizer.AuthorizeToken(token, room, role)
		if !d1.Allowed {
			return d1
		}
		d2 := a.Authorizer.AuthorizePeerCert(peerCertFromHTTP(r), room, role)
		if !d2.Allowed {
			d2.Principal = d1.Principal
			return d2
		}
		return d1
	default:
		return auth.Decision{Allowed: false, Reason: "unknown auth mode"}
	}
}

func peerCertFromHTTP(r *http.Request) *x509.Certificate {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	return r.TLS.PeerCertificates[0]
}
