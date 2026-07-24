// Package syncsvc implements multi-device synchronization for private items.
package syncsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// Service returns incremental item changes for an owner.
type Service struct {
	items storage.ItemRepository
}

// NewService constructs a sync Service.
func NewService(items storage.ItemRepository) *Service {
	return &Service{items: items}
}

// Result is a sync pull response.
type Result struct {
	Items         []model.Item
	ServerVersion int64
}

// Pull returns all changes for ownerID since the given cursors (including soft-deletes).
// Server version is the source of truth: clients must adopt returned items as-is.
func (s *Service) Pull(ctx context.Context, ownerID string, sinceVersion int64, sinceTimestamp time.Time) (*Result, error) {
	if ownerID == "" {
		return nil, fmt.Errorf("sync: missing owner")
	}

	items, err := s.items.ListChanged(ctx, ownerID, storage.SyncFilter{
		SinceVersion:   sinceVersion,
		SinceTimestamp: sinceTimestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("sync: list changed: %w", err)
	}

	serverVersion, err := s.items.MaxVersion(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("sync: max version: %w", err)
	}
	// Ensure server_version is at least the max among returned items.
	for i := range items {
		if items[i].Version > serverVersion {
			serverVersion = items[i].Version
		}
	}

	return &Result{Items: items, ServerVersion: serverVersion}, nil
}
