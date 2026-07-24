package data

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/pkg/secure"
)

// Service manages local vault items with E2E encryption.
type Service struct {
	vault storage.Vault
	key   *crypto.MasterKey
}

// NewService constructs a data Service.
func NewService(vault storage.Vault, key *crypto.MasterKey) *Service {
	return &Service{vault: vault, key: key}
}

// AddInput describes a new local item.
type AddInput struct {
	Type     model.DataType
	Plain    []byte
	Metadata model.Metadata
}

// Add encrypts and stores a new dirty local item.
// in.Plain is zeroed after encryption.
func (s *Service) Add(ctx context.Context, in AddInput) (storage.LocalItem, error) {
	if !in.Type.Valid() {
		return storage.LocalItem{}, fmt.Errorf("data: invalid type")
	}
	if len(in.Plain) == 0 {
		return storage.LocalItem{}, fmt.Errorf("data: empty payload")
	}
	sealed, err := s.key.Encrypt(in.Plain)
	secure.Zero(in.Plain)
	if err != nil {
		return storage.LocalItem{}, err
	}
	session, _ := s.vault.GetSession(ctx)
	item := storage.LocalItem{
		Item: model.Item{
			ID:            uuid.NewString(),
			OwnerID:       session.UserID,
			Type:          in.Type,
			EncryptedData: sealed,
			Metadata:      in.Metadata.Clone(),
			Version:       0,
			UpdatedAt:     time.Now().UTC(),
			Deleted:       false,
		},
		Dirty: true,
	}
	if err := s.vault.UpsertItem(ctx, item); err != nil {
		return storage.LocalItem{}, err
	}
	return item, nil
}

// Edit replaces plaintext of an existing item and marks it dirty.
func (s *Service) Edit(ctx context.Context, id string, plain []byte, metadata model.Metadata) (storage.LocalItem, error) {
	cur, err := s.vault.GetItem(ctx, id)
	if err != nil {
		return storage.LocalItem{}, err
	}
	if cur.Deleted {
		return storage.LocalItem{}, storage.ErrNotFound
	}
	sealed, err := s.key.Encrypt(plain)
	secure.Zero(plain)
	if err != nil {
		return storage.LocalItem{}, err
	}
	cur.EncryptedData = sealed
	if metadata != nil {
		cur.Metadata = metadata.Clone()
	}
	cur.UpdatedAt = time.Now().UTC()
	cur.Dirty = true
	if err := s.vault.UpsertItem(ctx, cur); err != nil {
		return storage.LocalItem{}, err
	}
	return cur, nil
}

// SoftDelete marks a local item deleted and dirty for sync.
func (s *Service) SoftDelete(ctx context.Context, id string) (storage.LocalItem, error) {
	cur, err := s.vault.GetItem(ctx, id)
	if err != nil {
		return storage.LocalItem{}, err
	}
	cur.Deleted = true
	cur.Dirty = true
	cur.UpdatedAt = time.Now().UTC()
	if err := s.vault.UpsertItem(ctx, cur); err != nil {
		return storage.LocalItem{}, err
	}
	return cur, nil
}

// Get decrypts and returns an item.
func (s *Service) Get(ctx context.Context, id string) (storage.LocalItem, []byte, error) {
	item, err := s.vault.GetItem(ctx, id)
	if err != nil {
		return storage.LocalItem{}, nil, err
	}
	plain, err := s.key.Decrypt(item.EncryptedData)
	if err != nil {
		return storage.LocalItem{}, nil, err
	}
	return item, plain, nil
}

// List returns local items (ciphertext only).
func (s *Service) List(ctx context.Context, typ *model.DataType, includeDeleted bool) ([]storage.LocalItem, error) {
	return s.vault.ListItems(ctx, typ, includeDeleted)
}
