// Package syncer performs bidirectional synchronization with the server.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/transport"
	"github.com/qutaq/gophkeeper/internal/model"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
)

// Service syncs local dirty items and pulls remote changes.
type Service struct {
	vault storage.Vault
	conn  *transport.Conn
}

// NewService constructs a sync Service.
func NewService(vault storage.Vault, conn *transport.Conn) *Service {
	return &Service{vault: vault, conn: conn}
}

// Result summarizes a sync run.
type Result struct {
	Pulled int
	Pushed int
}

// Sync pulls remote changes (server is source of truth by version), then pushes dirty locals.
func (s *Service) Sync(ctx context.Context, accessToken string) (*Result, error) {
	authCtx := transport.WithAccessToken(ctx, accessToken)
	since, err := s.vault.GetServerVersion(ctx)
	if err != nil {
		return nil, err
	}

	pull, err := s.conn.Sync.Sync(authCtx, &gophkeeperv1.SyncRequest{SinceVersion: since})
	if err != nil {
		return nil, fmt.Errorf("sync: pull: %w", err)
	}

	pulled := 0
	for _, remote := range pull.GetItems() {
		if err := s.applyRemote(ctx, remote); err != nil {
			return nil, err
		}
		pulled++
	}
	if err := s.vault.SetServerVersion(ctx, pull.GetServerVersion()); err != nil {
		return nil, err
	}

	dirty, err := s.vault.ListDirty(ctx)
	if err != nil {
		return nil, err
	}
	pushed := 0
	for _, item := range dirty {
		if err := s.pushOne(authCtx, ctx, item); err != nil {
			return nil, err
		}
		pushed++
	}

	// Refresh server version after pushes via another pull cursor bump from local max.
	session, _ := s.vault.GetSession(ctx)
	session.ServerVersion = pull.GetServerVersion()
	_ = s.vault.SaveSession(ctx, session)

	return &Result{Pulled: pulled, Pushed: pushed}, nil
}

func (s *Service) applyRemote(ctx context.Context, remote *gophkeeperv1.Item) error {
	local, err := s.vault.GetItem(ctx, remote.GetId())
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if err == nil {
		if local.Dirty && local.Version >= remote.GetVersion() {
			// Keep local dirty change; will push later.
			return nil
		}
		if remote.GetVersion() < local.Version {
			return nil
		}
		if local.Dirty && remote.GetVersion() > local.Version {
			log.Printf("sync: conflict item=%s remote_version=%d wins over dirty local", remote.GetId(), remote.GetVersion())
		}
	}

	item := storage.LocalItem{
		Item: model.Item{
			ID:            remote.GetId(),
			OwnerID:       remote.GetOwnerId(),
			Type:          model.DataType(remote.GetType()),
			EncryptedData: append([]byte(nil), remote.GetEncryptedData()...),
			Metadata:      model.Metadata(remote.GetMetadata()),
			Version:       remote.GetVersion(),
			Deleted:       remote.GetDeleted(),
		},
		Dirty: false,
	}
	if ts := remote.GetUpdatedAt(); ts != nil {
		item.UpdatedAt = ts.AsTime()
	}
	return s.vault.UpsertItem(ctx, item)
}

func (s *Service) pushOne(authCtx, ctx context.Context, item storage.LocalItem) error {
	switch {
	case item.Version == 0 && !item.Deleted:
		resp, err := s.conn.Data.AddItem(authCtx, &gophkeeperv1.AddItemRequest{
			Type:          gophkeeperv1.DataType(item.Type),
			EncryptedData: item.EncryptedData,
			Metadata:      map[string]string(item.Metadata),
		})
		if err != nil {
			return fmt.Errorf("sync: add %s: %w", item.ID, err)
		}
		return s.replaceAfterPush(ctx, item.ID, resp.GetItem())
	case item.Deleted && item.Version == 0:
		// Never synced — just drop locally.
		return s.vault.DeleteItemHard(ctx, item.ID)
	case item.Deleted:
		resp, err := s.conn.Data.DeleteItem(authCtx, &gophkeeperv1.DeleteItemRequest{Id: item.ID})
		if err != nil {
			return fmt.Errorf("sync: delete %s: %w", item.ID, err)
		}
		return s.replaceAfterPush(ctx, item.ID, resp.GetItem())
	default:
		resp, err := s.conn.Data.UpdateItem(authCtx, &gophkeeperv1.UpdateItemRequest{
			Id:              item.ID,
			EncryptedData:   item.EncryptedData,
			Metadata:        map[string]string(item.Metadata),
			ExpectedVersion: item.Version,
		})
		if err != nil {
			// Fall back to LWW.
			log.Printf("sync: update conflict item=%s, retrying LWW", item.ID)
			resp, err = s.conn.Data.UpdateItem(authCtx, &gophkeeperv1.UpdateItemRequest{
				Id:              item.ID,
				EncryptedData:   item.EncryptedData,
				Metadata:        map[string]string(item.Metadata),
				ExpectedVersion: 0,
			})
			if err != nil {
				return fmt.Errorf("sync: update %s: %w", item.ID, err)
			}
		}
		return s.replaceAfterPush(ctx, item.ID, resp.GetItem())
	}
}

func (s *Service) replaceAfterPush(ctx context.Context, oldID string, remote *gophkeeperv1.Item) error {
	if remote == nil {
		return fmt.Errorf("sync: empty server item")
	}
	if oldID != remote.GetId() {
		_ = s.vault.DeleteItemHard(ctx, oldID)
	}
	item := storage.LocalItem{
		Item: model.Item{
			ID:            remote.GetId(),
			OwnerID:       remote.GetOwnerId(),
			Type:          model.DataType(remote.GetType()),
			EncryptedData: append([]byte(nil), remote.GetEncryptedData()...),
			Metadata:      model.Metadata(remote.GetMetadata()),
			Version:       remote.GetVersion(),
			Deleted:       remote.GetDeleted(),
		},
		Dirty: false,
	}
	if ts := remote.GetUpdatedAt(); ts != nil {
		item.UpdatedAt = ts.AsTime()
	}
	if err := s.vault.UpsertItem(ctx, item); err != nil {
		return err
	}
	ver, _ := s.vault.GetServerVersion(ctx)
	if item.Version > ver {
		return s.vault.SetServerVersion(ctx, item.Version)
	}
	return nil
}
