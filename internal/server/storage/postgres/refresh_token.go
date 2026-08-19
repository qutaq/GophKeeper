package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// RefreshTokenRepository is a PostgreSQL RefreshTokenRepository.
type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

// Create inserts a refresh token row.
func (r *RefreshTokenRepository) Create(ctx context.Context, token *storage.RefreshToken) error {
	if token == nil {
		return fmt.Errorf("postgres: create refresh token: nil token")
	}
	if token.ID == "" {
		token.ID = uuid.NewString()
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}

	const q = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, q,
		token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.RevokedAt, token.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrConflict
		}
		return fmt.Errorf("postgres: create refresh token: %w", err)
	}
	return nil
}

// GetByHash returns a refresh token by its hash.
func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*storage.RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`
	row := r.pool.QueryRow(ctx, q, tokenHash)
	var t storage.RefreshToken
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get refresh token: %w", err)
	}
	return &t, nil
}

// Revoke marks a refresh token as revoked.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, tokenHash string) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, tokenHash)
	if err != nil {
		return fmt.Errorf("postgres: revoke refresh token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// RevokeAllForUser revokes every active refresh token for a user.
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("postgres: revoke user refresh tokens: %w", err)
	}
	return nil
}

var _ storage.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
