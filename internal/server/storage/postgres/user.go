package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// UserRepository is a PostgreSQL UserRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

// Create inserts a new user. If user.ID is empty, a UUID is generated.
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	if user == nil {
		return fmt.Errorf("postgres: create user: nil user")
	}
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}

	const q = `
		INSERT INTO users (id, login, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q,
		user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrConflict
		}
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

// GetByID returns a user by primary key.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	const q = `
		SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE id = $1`
	return r.scanUser(ctx, q, id)
}

// GetByLogin returns a user by unique login.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*model.User, error) {
	const q = `
		SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE login = $1`
	return r.scanUser(ctx, q, login)
}

func (r *UserRepository) scanUser(ctx context.Context, q string, arg any) (*model.User, error) {
	row := r.pool.QueryRow(ctx, q, arg)
	var u model.User
	err := row.Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get user: %w", err)
	}
	return &u, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Ensure interface compliance.
var _ storage.UserRepository = (*UserRepository)(nil)
