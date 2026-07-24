// Package sqlite implements local vault storage on SQLite (pure Go).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/model"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed Vault.
type Store struct {
	db *sql.DB
}

// Open creates/opens a vault database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sqlite: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL DEFAULT '',
  type INTEGER NOT NULL,
  encrypted_data BLOB NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  deleted INTEGER NOT NULL DEFAULT 0,
  dirty INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_items_dirty ON items(dirty);
CREATE INDEX IF NOT EXISTS idx_items_type ON items(type);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Now returns UTC now.
func (s *Store) Now() time.Time { return time.Now().UTC() }

// InitVault stores salt/verifier for a new vault.
func (s *Store) InitVault(ctx context.Context, meta storage.VaultMeta) error {
	if _, err := s.GetVaultMeta(ctx); err == nil {
		return storage.ErrAlreadyInitialized
	} else if !errors.Is(err, storage.ErrNotInitialized) {
		return err
	}
	if err := s.putMeta(ctx, "salt", meta.Salt); err != nil {
		return err
	}
	return s.putMeta(ctx, "verifier", []byte(meta.Verifier))
}

// GetVaultMeta loads salt/verifier.
func (s *Store) GetVaultMeta(ctx context.Context) (storage.VaultMeta, error) {
	salt, err := s.getMeta(ctx, "salt")
	if errors.Is(err, sql.ErrNoRows) {
		return storage.VaultMeta{}, storage.ErrNotInitialized
	}
	if err != nil {
		return storage.VaultMeta{}, err
	}
	ver, err := s.getMeta(ctx, "verifier")
	if err != nil {
		return storage.VaultMeta{}, err
	}
	return storage.VaultMeta{Salt: salt, Verifier: string(ver)}, nil
}

// SaveSession persists auth/sync session fields.
func (s *Store) SaveSession(ctx context.Context, session storage.Session) error {
	if session.AccessToken == nil {
		session.AccessToken = []byte{}
	}
	if session.RefreshToken == nil {
		session.RefreshToken = []byte{}
	}
	pairs := map[string][]byte{
		"user_id":        []byte(session.UserID),
		"login":          []byte(session.Login),
		"access_token":   session.AccessToken,
		"refresh_token":  session.RefreshToken,
		"server_version": []byte(fmt.Sprintf("%d", session.ServerVersion)),
	}
	for k, v := range pairs {
		if err := s.putMeta(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetSession loads the session (empty tokens if missing).
func (s *Store) GetSession(ctx context.Context) (storage.Session, error) {
	var out storage.Session
	out.UserID, _ = s.getMetaString(ctx, "user_id")
	out.Login, _ = s.getMetaString(ctx, "login")
	if v, err := s.getMeta(ctx, "access_token"); err == nil {
		out.AccessToken = v
	}
	if v, err := s.getMeta(ctx, "refresh_token"); err == nil {
		out.RefreshToken = v
	}
	if v, err := s.getMetaString(ctx, "server_version"); err == nil && v != "" {
		_, _ = fmt.Sscanf(v, "%d", &out.ServerVersion)
	}
	return out, nil
}

// ClearSession removes auth tokens but keeps vault meta and items.
func (s *Store) ClearSession(ctx context.Context) error {
	for _, k := range []string{"access_token", "refresh_token"} {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM meta WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return nil
}

// UpsertItem inserts or replaces a local item.
func (s *Store) UpsertItem(ctx context.Context, item storage.LocalItem) error {
	metaJSON, err := json.Marshal(item.Metadata)
	if err != nil {
		return err
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = s.Now()
	}
	dirty := 0
	if item.Dirty {
		dirty = 1
	}
	deleted := 0
	if item.Deleted {
		deleted = 1
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO items (id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted, dirty)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  owner_id=excluded.owner_id,
		  type=excluded.type,
		  encrypted_data=excluded.encrypted_data,
		  metadata=excluded.metadata,
		  version=excluded.version,
		  updated_at=excluded.updated_at,
		  deleted=excluded.deleted,
		  dirty=excluded.dirty`,
		item.ID, item.OwnerID, int(item.Type), item.EncryptedData, string(metaJSON),
		item.Version, item.UpdatedAt.UTC().Format(time.RFC3339Nano), deleted, dirty,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upsert item: %w", err)
	}
	return nil
}

// GetItem returns one local item.
func (s *Store) GetItem(ctx context.Context, id string) (storage.LocalItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted, dirty
		FROM items WHERE id = ?`, id)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.LocalItem{}, storage.ErrNotFound
	}
	return item, err
}

// ListItems lists local items.
func (s *Store) ListItems(ctx context.Context, typ *model.DataType, includeDeleted bool) ([]storage.LocalItem, error) {
	q := `SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted, dirty FROM items WHERE 1=1`
	args := make([]any, 0, 2)
	if !includeDeleted {
		q += ` AND deleted = 0`
	}
	if typ != nil {
		q += ` AND type = ?`
		args = append(args, int(*typ))
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectItems(rows)
}

// ListDirty returns items pending push.
func (s *Store) ListDirty(ctx context.Context) ([]storage.LocalItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted, dirty
		FROM items WHERE dirty = 1 ORDER BY updated_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectItems(rows)
}

// DeleteItemHard removes a row (rare; soft-delete is preferred).
func (s *Store) DeleteItemHard(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// SetServerVersion stores sync cursor.
func (s *Store) SetServerVersion(ctx context.Context, version int64) error {
	return s.putMeta(ctx, "server_version", []byte(fmt.Sprintf("%d", version)))
}

// GetServerVersion loads sync cursor.
func (s *Store) GetServerVersion(ctx context.Context) (int64, error) {
	v, err := s.getMetaString(ctx, "server_version")
	if err != nil || v == "" {
		return 0, nil
	}
	var n int64
	_, _ = fmt.Sscanf(v, "%d", &n)
	return n, nil
}

func (s *Store) putMeta(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) getMeta(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v, err
}

func (s *Store) getMetaString(ctx context.Context, key string) (string, error) {
	v, err := s.getMeta(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return string(v), err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (storage.LocalItem, error) {
	var (
		item     storage.LocalItem
		typ      int
		metaJSON string
		updated  string
		deleted  int
		dirty    int
	)
	err := row.Scan(&item.ID, &item.OwnerID, &typ, &item.EncryptedData, &metaJSON, &item.Version, &updated, &deleted, &dirty)
	if err != nil {
		return storage.LocalItem{}, err
	}
	item.Type = model.DataType(typ)
	item.Deleted = deleted == 1
	item.Dirty = dirty == 1
	if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		item.UpdatedAt = t
	}
	_ = json.Unmarshal([]byte(metaJSON), &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = model.Metadata{}
	}
	return item, nil
}

func collectItems(rows *sql.Rows) ([]storage.LocalItem, error) {
	out := make([]storage.LocalItem, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

var _ storage.Vault = (*Store)(nil)
