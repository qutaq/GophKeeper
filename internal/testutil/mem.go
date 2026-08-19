// Package testutil provides shared in-memory repositories for unit and integration tests.
package testutil

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// MemItems is an in-memory ItemRepository.
type MemItems struct {
	mu    sync.Mutex
	Items map[string]*model.Item
}

// NewMemItems constructs an empty MemItems store.
func NewMemItems() *MemItems {
	return &MemItems{Items: map[string]*model.Item{}}
}

func (m *MemItems) nextVersion(ownerID string) int64 {
	var max int64
	for _, it := range m.Items {
		if it.OwnerID == ownerID && it.Version > max {
			max = it.Version
		}
	}
	return max + 1
}

// Create implements storage.ItemRepository.
func (m *MemItems) Create(_ context.Context, item *model.Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	item.Version = m.nextVersion(item.OwnerID)
	item.UpdatedAt = time.Now().UTC()
	item.Deleted = false
	m.Items[item.ID] = cloneItem(item)
	return nil
}

// Update implements storage.ItemRepository.
func (m *MemItems) Update(_ context.Context, item *model.Item, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Items[item.ID]
	if !ok || cur.OwnerID != item.OwnerID || cur.Deleted {
		return storage.ErrNotFound
	}
	if cur.Version != expectedVersion {
		return storage.ErrVersionMismatch
	}
	item.Version = m.nextVersion(item.OwnerID)
	item.UpdatedAt = time.Now().UTC()
	item.Deleted = false
	m.Items[item.ID] = cloneItem(item)
	return nil
}

// ForceUpdate implements storage.ItemRepository.
func (m *MemItems) ForceUpdate(_ context.Context, item *model.Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Items[item.ID]
	if !ok || cur.OwnerID != item.OwnerID || cur.Deleted {
		return storage.ErrNotFound
	}
	item.Version = m.nextVersion(item.OwnerID)
	item.UpdatedAt = time.Now().UTC()
	item.Deleted = false
	m.Items[item.ID] = cloneItem(item)
	return nil
}

// SoftDelete implements storage.ItemRepository.
func (m *MemItems) SoftDelete(_ context.Context, ownerID, id string) (*model.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Items[id]
	if !ok || cur.OwnerID != ownerID || cur.Deleted {
		return nil, storage.ErrNotFound
	}
	cur.Deleted = true
	cur.Version = m.nextVersion(ownerID)
	cur.UpdatedAt = time.Now().UTC()
	return cloneItem(cur), nil
}

// GetByID implements storage.ItemRepository.
func (m *MemItems) GetByID(_ context.Context, ownerID, id string) (*model.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.Items[id]
	if !ok || cur.OwnerID != ownerID {
		return nil, storage.ErrNotFound
	}
	return cloneItem(cur), nil
}

// List implements storage.ItemRepository.
func (m *MemItems) List(_ context.Context, ownerID string, filter storage.ListItemsFilter) ([]model.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Item, 0)
	for _, it := range m.Items {
		if it.OwnerID != ownerID {
			continue
		}
		if !filter.IncludeDeleted && it.Deleted {
			continue
		}
		if filter.Type != nil && it.Type != *filter.Type {
			continue
		}
		out = append(out, *cloneItem(it))
	}
	return out, nil
}

// ListChanged implements storage.ItemRepository.
func (m *MemItems) ListChanged(_ context.Context, ownerID string, filter storage.SyncFilter) ([]model.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Item, 0)
	for _, it := range m.Items {
		if it.OwnerID != ownerID {
			continue
		}
		if filter.SinceVersion > 0 && it.Version <= filter.SinceVersion {
			continue
		}
		if !filter.SinceTimestamp.IsZero() && !it.UpdatedAt.After(filter.SinceTimestamp) {
			continue
		}
		out = append(out, *cloneItem(it))
	}
	return out, nil
}

// MaxVersion implements storage.ItemRepository.
func (m *MemItems) MaxVersion(_ context.Context, ownerID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var max int64
	for _, it := range m.Items {
		if it.OwnerID == ownerID && it.Version > max {
			max = it.Version
		}
	}
	return max, nil
}

func cloneItem(item *model.Item) *model.Item {
	cp := *item
	cp.EncryptedData = append([]byte(nil), item.EncryptedData...)
	cp.Metadata = item.Metadata.Clone()
	return &cp
}

// MemUsers is an in-memory UserRepository.
type MemUsers struct {
	mu    sync.Mutex
	ByID  map[string]*model.User
	ByLog map[string]*model.User
}

// NewMemUsers constructs an empty MemUsers store.
func NewMemUsers() *MemUsers {
	return &MemUsers{
		ByID:  map[string]*model.User{},
		ByLog: map[string]*model.User{},
	}
}

// Create implements storage.UserRepository.
func (m *MemUsers) Create(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.ByLog[user.Login]; ok {
		return storage.ErrConflict
	}
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	cp := *user
	m.ByID[user.ID] = &cp
	m.ByLog[user.Login] = &cp
	return nil
}

// GetByID implements storage.UserRepository.
func (m *MemUsers) GetByID(_ context.Context, id string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.ByID[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

// GetByLogin implements storage.UserRepository.
func (m *MemUsers) GetByLogin(_ context.Context, login string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.ByLog[login]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

// MemRefresh is an in-memory RefreshTokenRepository.
type MemRefresh struct {
	mu     sync.Mutex
	ByHash map[string]*storage.RefreshToken
}

// NewMemRefresh constructs an empty MemRefresh store.
func NewMemRefresh() *MemRefresh {
	return &MemRefresh{ByHash: map[string]*storage.RefreshToken{}}
}

// Create implements storage.RefreshTokenRepository.
func (m *MemRefresh) Create(_ context.Context, token *storage.RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token.ID == "" {
		token.ID = uuid.NewString()
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	cp := *token
	m.ByHash[token.TokenHash] = &cp
	return nil
}

// GetByHash implements storage.RefreshTokenRepository.
func (m *MemRefresh) GetByHash(_ context.Context, tokenHash string) (*storage.RefreshToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.ByHash[tokenHash]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *tok
	if tok.RevokedAt != nil {
		t := *tok.RevokedAt
		cp.RevokedAt = &t
	}
	return &cp, nil
}

// Revoke implements storage.RefreshTokenRepository.
func (m *MemRefresh) Revoke(_ context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.ByHash[tokenHash]
	if !ok {
		return storage.ErrNotFound
	}
	now := time.Now().UTC()
	tok.RevokedAt = &now
	return nil
}

// RevokeAllForUser implements storage.RefreshTokenRepository.
func (m *MemRefresh) RevokeAllForUser(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, tok := range m.ByHash {
		if tok.UserID == userID && tok.RevokedAt == nil {
			tok.RevokedAt = &now
		}
	}
	return nil
}

var (
	_ storage.ItemRepository         = (*MemItems)(nil)
	_ storage.UserRepository         = (*MemUsers)(nil)
	_ storage.RefreshTokenRepository = (*MemRefresh)(nil)
)
