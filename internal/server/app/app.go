// Package app wires and runs the GophKeeper server.
package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/qutaq/gophkeeper/internal/config"
	"github.com/qutaq/gophkeeper/internal/server/auth"
	"github.com/qutaq/gophkeeper/internal/server/data"
	grpcserver "github.com/qutaq/gophkeeper/internal/server/grpc"
	"github.com/qutaq/gophkeeper/internal/server/storage/postgres"
	syncsvc "github.com/qutaq/gophkeeper/internal/server/sync"
	"github.com/qutaq/gophkeeper/pkg/crypto/atrest"
)

// Run loads config, migrates DB, and serves gRPC until signal.
func Run(ctx context.Context) error {
	cfg, err := config.LoadServer()
	if err != nil {
		return err
	}

	if err := postgres.MigrateUp(postgres.ToMigrateURL(cfg.DatabaseURL), cfg.MigrationsDir); err != nil {
		return fmt.Errorf("app: migrate: %w", err)
	}

	cipher, err := atrest.NewAESGCMFromHex(cfg.AtRestKeyHex)
	if err != nil {
		return fmt.Errorf("app: at-rest key: %w", err)
	}

	store, err := postgres.Open(ctx, cfg.DatabaseURL, cipher)
	if err != nil {
		return fmt.Errorf("app: db: %w", err)
	}
	defer store.Close()

	keys, err := auth.ParseKeyRingEnv(cfg.JWTKeys, cfg.JWTCurrentKID)
	if err != nil {
		return fmt.Errorf("app: jwt keys: %w", err)
	}
	tokens, err := auth.NewTokenManager(auth.TokenManagerConfig{
		Keys:       keys,
		Issuer:     cfg.JWTIssuer,
		AccessTTL:  cfg.AccessTTL,
		RefreshTTL: cfg.RefreshTTL,
	})
	if err != nil {
		return fmt.Errorf("app: token manager: %w", err)
	}

	authSvc := auth.NewService(store.Users, store.RefreshTokens, tokens)
	dataSvc := data.NewService(store.Items)
	syncService := syncsvc.NewService(store.Items)

	var tlsCreds credentials.TransportCredentials
	if cfg.TLSCertFile != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("app: tls: %w", err)
		}
		tlsCreds = creds
	}

	server := grpcserver.New(grpcserver.ServerConfig{
		Auth:             authSvc,
		Data:             dataSvc,
		Sync:             syncService,
		EnableReflection: cfg.EnableReflect,
		TLSCreds:         tlsCreds,
	})

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("app: listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		mode := "plaintext"
		if tlsCreds != nil {
			mode = "TLS"
		}
		log.Printf("gophkeeper-server listening on %s (%s)", cfg.GRPCAddr, mode)
		errCh <- server.Serve(lis)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		server.GracefulStop()
		return ctx.Err()
	case sig := <-sigCh:
		log.Printf("signal %v, shutting down", sig)
		server.GracefulStop()
		return nil
	case err := <-errCh:
		if err != nil && err != grpc.ErrServerStopped {
			return err
		}
		return nil
	}
}
