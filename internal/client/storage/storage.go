// Package storage defines the local vault persistence interface.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
)

var (
	// ErrNotFound is returned when a local entity is missing.
	ErrNotFound = errors.New("client/storage: not found")
	// ErrNotInitialized is returned when the vault has no master-key material yet.
	ErrNotInitialized = errors.New("client/storage: vault not initialized")
	// ErrAlreadyInitialized is returned when InitVault is called twice.
	ErrAlreadyInitialized = errors.New("client/storage: vault already initialized")
)

// Session holds auth tokens and sync cursor persisted locally.
// AccessToken and RefreshToken are ciphertext sealed with the master key.
type Session struct {
	UserID        string
	Login         string
	AccessToken   []byte
	RefreshToken  []byte
	ServerVersion int64
}

// VaultMeta is master-key material stored in the local DB.
type VaultMeta struct {
	Salt     []byte
	Verifier string
}

// LocalItem is a vault record with sync bookkeeping.
type LocalItem struct {
	model.Item
	Dirty bool
}

// Vault is the local encrypted vault repository.
type Vault interface {
	Close() error

	InitVault(ctx context.Context, meta VaultMeta) error
	GetVaultMeta(ctx context.Context) (VaultMeta, error)

	SaveSession(ctx context.Context, session Session) error
	GetSession(ctx context.Context) (Session, error)
	ClearSession(ctx context.Context) error

	UpsertItem(ctx context.Context, item LocalItem) error
	GetItem(ctx context.Context, id string) (LocalItem, error)
	ListItems(ctx context.Context, typ *model.DataType, includeDeleted bool) ([]LocalItem, error)
	ListDirty(ctx context.Context) ([]LocalItem, error)
	DeleteItemHard(ctx context.Context, id string) error

	SetServerVersion(ctx context.Context, version int64) error
	GetServerVersion(ctx context.Context) (int64, error)

	// Used by sync bookkeeping timestamps if needed by callers.
	Now() time.Time
}
