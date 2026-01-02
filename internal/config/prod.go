package config

type AdminConfig struct {
	Addr        string `yaml:"addr"`
	EnablePprof bool   `yaml:"enable_pprof"`
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`

	CAFile            string `yaml:"ca_file"`
	ClientCAFile      string `yaml:"client_ca_file"`
	RequireClientCert bool   `yaml:"require_client_cert"`

	ServerName         string `yaml:"server_name"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}
