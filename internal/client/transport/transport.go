// Package transport provides a gRPC connection helper for the CLI client.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/qutaq/gophkeeper/internal/config"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
)

// Conn wraps a gRPC client connection and generated stubs.
type Conn struct {
	raw  *grpc.ClientConn
	Auth gophkeeperv1.AuthServiceClient
	Data gophkeeperv1.DataServiceClient
	Sync gophkeeperv1.SyncServiceClient
}

// Dial opens a TLS gRPC connection using client config.
// Plaintext is not supported: tokens and ciphertext must not travel unprotected.
func Dial(cfg config.Client) (*Conn, error) {
	tlsCfg, err := buildTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	creds := credentials.NewTLS(tlsCfg)

	raw, err := grpc.NewClient(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("transport: dial: %w", err)
	}
	return &Conn{
		raw:  raw,
		Auth: gophkeeperv1.NewAuthServiceClient(raw),
		Data: gophkeeperv1.NewDataServiceClient(raw),
		Sync: gophkeeperv1.NewSyncServiceClient(raw),
	}, nil
}

func buildTLSConfig(c config.ClientTLS) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("transport: read ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("transport: invalid ca pem")
		}
		tlsCfg.RootCAs = pool
	}
	if c.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("transport: client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	tlsCfg.InsecureSkipVerify = c.InsecureSkipVerify //nolint:gosec // optional for local/dev self-signed
	return tlsCfg, nil
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Close()
}

// WithAccessToken attaches a Bearer access token to outgoing context.
func WithAccessToken(ctx context.Context, accessToken string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
}
