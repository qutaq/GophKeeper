// Package postgres implements storage repositories on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qutaq/gophkeeper/internal/server/storage"
	"github.com/qutaq/gophkeeper/pkg/crypto/atrest"
)

// Store groups PostgreSQL-backed repositories.
type Store struct {
	pool          *pgxpool.Pool
	cipher        atrest.Cipher
	Users         storage.UserRepository
	Items         storage.ItemRepository
	RefreshTokens storage.RefreshTokenRepository
}

// Open connects to PostgreSQL and wires repositories.
// cipher encrypts item payloads at rest (may be nil only in tests that never touch items).
func Open(ctx context.Context, databaseURL string, cipher atrest.Cipher) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	s := &Store{pool: pool, cipher: cipher}
	s.Users = &UserRepository{pool: pool}
	s.Items = &ItemRepository{pool: pool, cipher: cipher}
	s.RefreshTokens = &RefreshTokenRepository{pool: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// MigrateUp applies all up migrations from migrationsDir (filesystem path).
// databaseURL must be a pgx5 URL, e.g. "pgx5://user:pass@localhost:5432/db?sslmode=disable".
func MigrateUp(databaseURL, migrationsDir string) error {
	sourceURL, err := fileSourceURL(migrationsDir)
	if err != nil {
		return fmt.Errorf("postgres: migrate source: %w", err)
	}
	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: migrate open: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("postgres: migrate up: %w", err)
	}
	return nil
}

func fileSourceURL(migrationsDir string) (string, error) {
	abs, err := filepath.Abs(migrationsDir)
	if err != nil {
		return "", err
	}
	abs = filepath.ToSlash(abs)
	// Windows needs file:///C:/...; Unix uses file:///path or file://path.
	if runtime.GOOS == "windows" {
		return "file:///" + abs, nil
	}
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs, nil
}

// ToMigrateURL converts a standard postgres DSN into a golang-migrate pgx5 URL.
// Accepts postgres:// or postgresql:// schemes.
func ToMigrateURL(databaseURL string) string {
	switch {
	case strings.HasPrefix(databaseURL, "postgres://"):
		return "pgx5://" + strings.TrimPrefix(databaseURL, "postgres://")
	case strings.HasPrefix(databaseURL, "postgresql://"):
		return "pgx5://" + strings.TrimPrefix(databaseURL, "postgresql://")
	default:
		return databaseURL
	}
}
