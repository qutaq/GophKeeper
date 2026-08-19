// Package app wires and runs the GophKeeper server.
package app

import (
	"context"
	"fmt"
	"log/slog"
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	tlsCreds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return fmt.Errorf("app: tls: %w", err)
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
		slog.Info("gophkeeper-server listening", "addr", cfg.GRPCAddr, "mode", "TLS")
		errCh <- server.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		stop()
		slog.Info("shutting down", "cause", context.Cause(ctx))
		server.GracefulStop()
		return nil
	case err := <-errCh:
		stop()
		if err != nil && err != grpc.ErrServerStopped {
			return err
		}
		return nil
	}
}
