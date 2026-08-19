// Package config loads server and client configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Server holds server runtime configuration.
type Server struct {
	GRPCAddr      string
	DatabaseURL   string
	MigrationsDir string
	AtRestKeyHex  string
	JWTKeys       string
	JWTCurrentKID string
	JWTIssuer     string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	EnableReflect bool
	// TLSCertFile / TLSKeyFile are required for gRPC TLS (use self-signed in dev).
	TLSCertFile string
	TLSKeyFile  string
}

// LoadServer reads server config from environment variables.
func LoadServer() (Server, error) {
	cfg := Server{
		GRPCAddr:      getenv("GOPHKEEPER_GRPC_ADDR", ":50051"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		MigrationsDir: getenv("GOPHKEEPER_MIGRATIONS_DIR", "migrations"),
		AtRestKeyHex:  os.Getenv("GOPHKEEPER_ATREST_KEY"),
		JWTKeys:       os.Getenv("GOPHKEEPER_JWT_KEYS"),
		JWTCurrentKID: os.Getenv("GOPHKEEPER_JWT_CURRENT_KID"),
		JWTIssuer:     getenv("GOPHKEEPER_JWT_ISSUER", "gophkeeper"),
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		EnableReflect: getenvBool("GOPHKEEPER_GRPC_REFLECTION", true),
		TLSCertFile:   os.Getenv("GOPHKEEPER_TLS_CERT"),
		TLSKeyFile:    os.Getenv("GOPHKEEPER_TLS_KEY"),
	}
	if v := os.Getenv("GOPHKEEPER_ACCESS_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Server{}, fmt.Errorf("config: GOPHKEEPER_ACCESS_TTL: %w", err)
		}
		if d <= 0 {
			return Server{}, fmt.Errorf("config: GOPHKEEPER_ACCESS_TTL must be positive")
		}
		cfg.AccessTTL = d
	}
	if v := os.Getenv("GOPHKEEPER_REFRESH_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Server{}, fmt.Errorf("config: GOPHKEEPER_REFRESH_TTL: %w", err)
		}
		if d <= 0 {
			return Server{}, fmt.Errorf("config: GOPHKEEPER_REFRESH_TTL must be positive")
		}
		cfg.RefreshTTL = d
	}
	if cfg.DatabaseURL == "" {
		return Server{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.AtRestKeyHex == "" {
		return Server{}, fmt.Errorf("config: GOPHKEEPER_ATREST_KEY is required (64 hex chars)")
	}
	if cfg.JWTKeys == "" {
		return Server{}, fmt.Errorf("config: GOPHKEEPER_JWT_KEYS is required (kid:hex[,kid:hex...])")
	}
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return Server{}, fmt.Errorf("config: GOPHKEEPER_TLS_CERT and GOPHKEEPER_TLS_KEY are required (use self-signed certs in dev)")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
