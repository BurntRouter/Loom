package config

type AuthMode string

const (
	AuthModeDisabled AuthMode = "disabled"
	AuthModeToken    AuthMode = "token"
	AuthModeMTLS     AuthMode = "mtls"
	AuthModeBoth     AuthMode = "both"
)

type AuthConfig struct {
	Mode AuthMode `yaml:"mode"`

	Tokens []TokenConfig `yaml:"tokens"`
	Certs  []CertConfig  `yaml:"certs"`
}

type TokenConfig struct {
	Token     string   `yaml:"token"`
	Principal string   `yaml:"principal"`
	Rooms     []string `yaml:"rooms"`
	Roles     []string `yaml:"roles"`
}

type CertConfig struct {
	Principal string   `yaml:"principal"`
	Rooms     []string `yaml:"rooms"`
	Roles     []string `yaml:"roles"`
}
