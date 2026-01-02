package auth

import (
	"strings"

	"github.com/BurntRouter/Loom/internal/config"
)

func FromConfig(cfg config.AuthConfig) *Authorizer {
	var tokenRules []TokenRule
	for _, t := range cfg.Tokens {
		var roles []Role
		for _, r := range t.Roles {
			roles = append(roles, Role(strings.ToLower(r)))
		}
		tokenRules = append(tokenRules, TokenRule{
			Token:     t.Token,
			Principal: t.Principal,
			Rooms:     t.Rooms,
			Roles:     roles,
		})
	}

	var certRules []CertRule
	for _, c := range cfg.Certs {
		var roles []Role
		for _, r := range c.Roles {
			roles = append(roles, Role(strings.ToLower(r)))
		}
		certRules = append(certRules, CertRule{
			Principal: c.Principal,
			Rooms:     c.Rooms,
			Roles:     roles,
		})
	}

	return New(tokenRules, certRules)
}
