package auth

import (
	"crypto/x509"
	"errors"
)

type Role string

const (
	RoleProduce Role = "produce"
	RoleConsume Role = "consume"
)

type Decision struct {
	Allowed   bool
	Principal string
	Reason    string
}

type TokenRule struct {
	Token     string
	Principal string
	Rooms     []string
	Roles     []Role
}

type CertRule struct {
	Principal string
	Rooms     []string
	Roles     []Role
}

type Authorizer struct {
	tokensByValue map[string]TokenRule
	certsByName   map[string]CertRule
}

func New(tokenRules []TokenRule, certRules []CertRule) *Authorizer {
	a := &Authorizer{
		tokensByValue: make(map[string]TokenRule, len(tokenRules)),
		certsByName:   make(map[string]CertRule, len(certRules)),
	}
	for _, r := range tokenRules {
		a.tokensByValue[r.Token] = r
	}
	for _, r := range certRules {
		a.certsByName[r.Principal] = r
	}
	return a
}

func (a *Authorizer) AuthorizeToken(token string, room string, role Role) Decision {
	if token == "" {
		return Decision{Allowed: false, Reason: "missing token"}
	}
	r, ok := a.tokensByValue[token]
	if !ok {
		return Decision{Allowed: false, Reason: "invalid token"}
	}
	if !roleAllowed(role, r.Roles) {
		return Decision{Allowed: false, Principal: r.Principal, Reason: "role not allowed"}
	}
	if !roomAllowed(room, r.Rooms) {
		return Decision{Allowed: false, Principal: r.Principal, Reason: "room not allowed"}
	}
	return Decision{Allowed: true, Principal: r.Principal}
}

func (a *Authorizer) AuthorizePeerCert(cert *x509.Certificate, room string, role Role) Decision {
	if cert == nil {
		return Decision{Allowed: false, Reason: "missing peer certificate"}
	}
	principal := cert.Subject.CommonName
	if principal == "" {
		return Decision{Allowed: false, Reason: "missing certificate CN"}
	}
	r, ok := a.certsByName[principal]
	if !ok {
		return Decision{Allowed: false, Principal: principal, Reason: "unknown principal"}
	}
	if !roleAllowed(role, r.Roles) {
		return Decision{Allowed: false, Principal: principal, Reason: "role not allowed"}
	}
	if !roomAllowed(room, r.Rooms) {
		return Decision{Allowed: false, Principal: principal, Reason: "room not allowed"}
	}
	return Decision{Allowed: true, Principal: principal}
}

func roomAllowed(room string, rooms []string) bool {
	if len(rooms) == 0 {
		return false
	}
	for _, r := range rooms {
		if r == "*" || r == room {
			return true
		}
	}
	return false
}

func roleAllowed(role Role, roles []Role) bool {
	if len(roles) == 0 {
		return false
	}
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

var ErrNoAuthConfigured = errors.New("auth: no rules configured")
