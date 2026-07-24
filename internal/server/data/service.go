package data

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// Service provides item CRUD operations scoped to an owner.
type Service struct {
	items storage.ItemRepository
}

// NewService constructs a data Service.
func NewService(items storage.ItemRepository) *Service {
	return &Service{items: items}
}

// AddInput is the payload for Add.
type AddInput struct {
	Type          model.DataType
	EncryptedData []byte
	Metadata      model.Metadata
}

// UpdateInput is the payload for Update.
type UpdateInput struct {
	ID              string
	EncryptedData   []byte
	Metadata        model.Metadata
	ExpectedVersion int64
}

// Add creates a new encrypted item for ownerID.
func (s *Service) Add(ctx context.Context, ownerID string, in AddInput) (*model.Item, error) {
	if ownerID == "" {
		return nil, fmt.Errorf("%w: missing owner", ErrInvalidInput)
	}
	if !in.Type.Valid() {
		return nil, fmt.Errorf("%w: invalid data type", ErrInvalidInput)
	}
	if len(in.EncryptedData) == 0 {
		return nil, fmt.Errorf("%w: empty encrypted payload", ErrInvalidInput)
	}

	item := &model.Item{
		OwnerID:       ownerID,
		Type:          in.Type,
		EncryptedData: append([]byte(nil), in.EncryptedData...),
		Metadata:      in.Metadata.Clone(),
	}
	if err := s.items.Create(ctx, item); err != nil {
		return nil, fmt.Errorf("data: add: %w", err)
	}
	return item, nil
}

// Update changes an existing item. expected_version > 0 enables OCC;
// expected_version == 0 applies last-write-wins overwrite and logs the conflict path.
func (s *Service) Update(ctx context.Context, ownerID string, in UpdateInput) (*model.Item, error) {
	if ownerID == "" || in.ID == "" {
		return nil, fmt.Errorf("%w: missing owner or id", ErrInvalidInput)
	}
	if len(in.EncryptedData) == 0 {
		return nil, fmt.Errorf("%w: empty encrypted payload", ErrInvalidInput)
	}

	current, err := s.items.GetByID(ctx, ownerID, in.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("data: get: %w", err)
	}
	if current.Deleted {
		return nil, ErrNotFound
	}

	item := &model.Item{
		ID:            current.ID,
		OwnerID:       ownerID,
		Type:          current.Type,
		EncryptedData: append([]byte(nil), in.EncryptedData...),
		Metadata:      in.Metadata.Clone(),
	}

	if in.ExpectedVersion == 0 {
		log.Printf("data: lww update owner=%s item=%s server_version=%d", ownerID, in.ID, current.Version)
		if err := s.items.ForceUpdate(ctx, item); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("data: lww update: %w", err)
		}
		return item, nil
	}

	if err := s.items.Update(ctx, item, in.ExpectedVersion); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		if errors.Is(err, storage.ErrVersionMismatch) {
			log.Printf(
				"data: conflict owner=%s item=%s expected=%d server_version=%d (server wins)",
				ownerID, in.ID, in.ExpectedVersion, current.Version,
			)
			return nil, fmt.Errorf("%w: server version %d", ErrConflict, current.Version)
		}
		return nil, fmt.Errorf("data: update: %w", err)
	}
	return item, nil
}

// Delete soft-deletes an item owned by ownerID.
func (s *Service) Delete(ctx context.Context, ownerID, id string) (*model.Item, error) {
	if ownerID == "" || id == "" {
		return nil, fmt.Errorf("%w: missing owner or id", ErrInvalidInput)
	}
	item, err := s.items.SoftDelete(ctx, ownerID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("data: delete: %w", err)
	}
	return item, nil
}

// Get returns one item owned by ownerID.
func (s *Service) Get(ctx context.Context, ownerID, id string) (*model.Item, error) {
	if ownerID == "" || id == "" {
		return nil, fmt.Errorf("%w: missing owner or id", ErrInvalidInput)
	}
	item, err := s.items.GetByID(ctx, ownerID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("data: get: %w", err)
	}
	return item, nil
}

// List returns items for ownerID.
func (s *Service) List(ctx context.Context, ownerID string, filter storage.ListItemsFilter) ([]model.Item, error) {
	if ownerID == "" {
		return nil, fmt.Errorf("%w: missing owner", ErrInvalidInput)
	}
	if filter.Type != nil && !filter.Type.Valid() {
		return nil, fmt.Errorf("%w: invalid data type", ErrInvalidInput)
	}
	items, err := s.items.List(ctx, ownerID, filter)
	if err != nil {
		return nil, fmt.Errorf("data: list: %w", err)
	}
	return items, nil
}
