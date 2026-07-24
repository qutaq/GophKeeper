package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Client holds CLI client configuration.
type Client struct {
	ServerAddress string    `yaml:"server_address"`
	TLS           ClientTLS `yaml:"tls"`
	DataDir       string    `yaml:"data_dir"`
}

// ClientTLS configures transport security for gRPC.
type ClientTLS struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// DefaultClient returns defaults for local development.
func DefaultClient() Client {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".gophkeeper")
	return Client{
		ServerAddress: getenv("GOPHKEEPER_SERVER", "localhost:50051"),
		TLS: ClientTLS{
			Enabled:            getenvBool("GOPHKEEPER_TLS", false),
			CAFile:             os.Getenv("GOPHKEEPER_TLS_CA"),
			CertFile:           os.Getenv("GOPHKEEPER_TLS_CLIENT_CERT"),
			KeyFile:            os.Getenv("GOPHKEEPER_TLS_CLIENT_KEY"),
			InsecureSkipVerify: getenvBool("GOPHKEEPER_TLS_SKIP_VERIFY", false),
		},
		DataDir: getenv("GOPHKEEPER_DATA_DIR", dataDir),
	}
}

// LoadClient loads client config from optional YAML path, then applies env overrides.
func LoadClient(path string) (Client, error) {
	cfg := DefaultClient()
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".gophkeeper", "config.yaml")
		}
	}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(raw, &cfg); err != nil {
				return Client{}, fmt.Errorf("config: parse %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return Client{}, fmt.Errorf("config: read %s: %w", path, err)
		}
	}
	// Env overrides always win when set.
	if v := os.Getenv("GOPHKEEPER_SERVER"); v != "" {
		cfg.ServerAddress = v
	}
	if v := os.Getenv("GOPHKEEPER_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if os.Getenv("GOPHKEEPER_TLS") != "" {
		cfg.TLS.Enabled = getenvBool("GOPHKEEPER_TLS", cfg.TLS.Enabled)
	}
	if v := os.Getenv("GOPHKEEPER_TLS_CA"); v != "" {
		cfg.TLS.CAFile = v
	}
	if v := os.Getenv("GOPHKEEPER_TLS_CLIENT_CERT"); v != "" {
		cfg.TLS.CertFile = v
	}
	if v := os.Getenv("GOPHKEEPER_TLS_CLIENT_KEY"); v != "" {
		cfg.TLS.KeyFile = v
	}
	if os.Getenv("GOPHKEEPER_TLS_SKIP_VERIFY") != "" {
		cfg.TLS.InsecureSkipVerify = getenvBool("GOPHKEEPER_TLS_SKIP_VERIFY", false)
	}
	if cfg.ServerAddress == "" {
		return Client{}, fmt.Errorf("config: server_address is required")
	}
	if cfg.DataDir == "" {
		return Client{}, fmt.Errorf("config: data_dir is required")
	}
	if (cfg.TLS.CertFile == "") != (cfg.TLS.KeyFile == "") {
		return Client{}, fmt.Errorf("config: tls.cert_file and tls.key_file must both be set")
	}
	return cfg, nil
}

// DBPath returns the SQLite database path inside DataDir.
func (c Client) DBPath() string {
	return filepath.Join(c.DataDir, "vault.db")
}
