// Package storage defines server-side persistence interfaces.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
)

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("storage: not found")
	// ErrConflict is returned on unique constraint violations (e.g. login taken).
	ErrConflict = errors.New("storage: conflict")
	// ErrVersionMismatch is returned when optimistic concurrency check fails.
	ErrVersionMismatch = errors.New("storage: version mismatch")
)

// UserRepository persists user accounts.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByLogin(ctx context.Context, login string) (*model.User, error)
}

// RefreshToken is a persisted refresh session.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// RefreshTokenRepository stores and revokes refresh tokens.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	Revoke(ctx context.Context, tokenHash string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}

// ListItemsFilter controls List queries.
type ListItemsFilter struct {
	Type           *model.DataType
	IncludeDeleted bool
}

// SyncFilter controls incremental sync queries.
type SyncFilter struct {
	SinceVersion   int64
	SinceTimestamp time.Time
}

// ItemRepository persists encrypted private data items.
type ItemRepository interface {
	Create(ctx context.Context, item *model.Item) error
	Update(ctx context.Context, item *model.Item, expectedVersion int64) error
	// ForceUpdate applies last-write-wins overwrite without checking expected version.
	ForceUpdate(ctx context.Context, item *model.Item) error
	SoftDelete(ctx context.Context, ownerID, id string) (*model.Item, error)
	GetByID(ctx context.Context, ownerID, id string) (*model.Item, error)
	List(ctx context.Context, ownerID string, filter ListItemsFilter) ([]model.Item, error)
	ListChanged(ctx context.Context, ownerID string, filter SyncFilter) ([]model.Item, error)
	MaxVersion(ctx context.Context, ownerID string) (int64, error)
}
