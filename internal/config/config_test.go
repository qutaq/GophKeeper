package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qutaq/gophkeeper/internal/config"
)

func TestLoadServerRequiresSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GOPHKEEPER_ATREST_KEY", "")
	t.Setenv("GOPHKEEPER_JWT_KEYS", "")
	if _, err := config.LoadServer(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadServerOK(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("GOPHKEEPER_ATREST_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_JWT_KEYS", "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_JWT_CURRENT_KID", "v1")
	t.Setenv("GOPHKEEPER_TLS_CERT", "")
	t.Setenv("GOPHKEEPER_TLS_KEY", "")
	cfg, err := config.LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GRPCAddr == "" || cfg.JWTIssuer == "" {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadServerTLSPairRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("GOPHKEEPER_ATREST_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_JWT_KEYS", "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_TLS_CERT", "/tmp/a.crt")
	t.Setenv("GOPHKEEPER_TLS_KEY", "")
	if _, err := config.LoadServer(); err == nil {
		t.Fatal("expected tls pair error")
	}
}

func TestLoadClientYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server_address: example:50051\ndata_dir: " + filepath.ToSlash(dir) + "\ntls:\n  enabled: true\n  ca_file: ca.pem\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPHKEEPER_SERVER", "")
	t.Setenv("GOPHKEEPER_DATA_DIR", "")
	t.Setenv("GOPHKEEPER_TLS", "")
	cfg, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddress != "example:50051" || !cfg.TLS.Enabled || cfg.TLS.CAFile != "ca.pem" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.DBPath() != filepath.Join(dir, "vault.db") {
		t.Fatalf("dbpath=%s", cfg.DBPath())
	}

	t.Setenv("GOPHKEEPER_SERVER", "env-host:1")
	cfg, err = config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddress != "env-host:1" {
		t.Fatalf("env override failed: %s", cfg.ServerAddress)
	}
}

func TestLoadServerTTLAndReflection(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("GOPHKEEPER_ATREST_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_JWT_KEYS", "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("GOPHKEEPER_ACCESS_TTL", "30m")
	t.Setenv("GOPHKEEPER_REFRESH_TTL", "48h")
	t.Setenv("GOPHKEEPER_GRPC_REFLECTION", "false")
	t.Setenv("GOPHKEEPER_TLS_CERT", "")
	t.Setenv("GOPHKEEPER_TLS_KEY", "")
	cfg, err := config.LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTTL.Minutes() != 30 || cfg.RefreshTTL.Hours() != 48 || cfg.EnableReflect {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadClientEnvTLSOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOPHKEEPER_SERVER", "srv:9")
	t.Setenv("GOPHKEEPER_DATA_DIR", dir)
	t.Setenv("GOPHKEEPER_TLS", "true")
	t.Setenv("GOPHKEEPER_TLS_CA", "ca.pem")
	t.Setenv("GOPHKEEPER_TLS_SKIP_VERIFY", "true")
	t.Setenv("GOPHKEEPER_TLS_CLIENT_CERT", "")
	t.Setenv("GOPHKEEPER_TLS_CLIENT_KEY", "")
	cfg, err := config.LoadClient(filepath.Join(dir, "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CAFile != "ca.pem" || !cfg.TLS.InsecureSkipVerify {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadClientTLSClientCertPair(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server_address: localhost:1\ndata_dir: "+filepath.ToSlash(dir)+"\ntls:\n  cert_file: only.crt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPHKEEPER_TLS_CLIENT_CERT", "")
	t.Setenv("GOPHKEEPER_TLS_CLIENT_KEY", "")
	if _, err := config.LoadClient(path); err == nil {
		t.Fatal("expected cert/key pair error")
	}
}
