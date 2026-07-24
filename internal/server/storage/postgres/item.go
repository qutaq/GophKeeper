package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
	"github.com/qutaq/gophkeeper/pkg/crypto/atrest"
)

// ItemRepository is a PostgreSQL ItemRepository with at-rest encryption.
type ItemRepository struct {
	pool   *pgxpool.Pool
	cipher atrest.Cipher
}

// Create inserts a new item. Version is assigned as max(owner)+1 inside a transaction.
func (r *ItemRepository) Create(ctx context.Context, item *model.Item) error {
	if item == nil {
		return fmt.Errorf("postgres: create item: nil item")
	}
	if r.cipher == nil {
		return fmt.Errorf("postgres: create item: cipher is nil")
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}

	sealed, err := r.cipher.Encrypt(item.EncryptedData)
	if err != nil {
		return fmt.Errorf("postgres: encrypt item: %w", err)
	}
	metaJSON, err := marshalMetadata(item.Metadata)
	if err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var nextVersion int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM items WHERE owner_id = $1`, item.OwnerID).Scan(&nextVersion)
	if err != nil {
		return fmt.Errorf("postgres: next version: %w", err)
	}
	item.Version = nextVersion
	item.Deleted = false

	const q = `
		INSERT INTO items (id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = tx.Exec(ctx, q,
		item.ID, item.OwnerID, int16(item.Type), sealed, metaJSON,
		item.Version, item.UpdatedAt, item.Deleted,
	)
	if err != nil {
		return fmt.Errorf("postgres: create item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// Update replaces payload/metadata when expectedVersion matches the current row.
func (r *ItemRepository) Update(ctx context.Context, item *model.Item, expectedVersion int64) error {
	return r.writeUpdate(ctx, item, &expectedVersion)
}

// ForceUpdate overwrites an item without optimistic concurrency (last-write-wins).
func (r *ItemRepository) ForceUpdate(ctx context.Context, item *model.Item) error {
	return r.writeUpdate(ctx, item, nil)
}

func (r *ItemRepository) writeUpdate(ctx context.Context, item *model.Item, expectedVersion *int64) error {
	if item == nil {
		return fmt.Errorf("postgres: update item: nil item")
	}
	if r.cipher == nil {
		return fmt.Errorf("postgres: update item: cipher is nil")
	}

	sealed, err := r.cipher.Encrypt(item.EncryptedData)
	if err != nil {
		return fmt.Errorf("postgres: encrypt item: %w", err)
	}
	metaJSON, err := marshalMetadata(item.Metadata)
	if err != nil {
		return err
	}
	item.UpdatedAt = time.Now().UTC()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var nextVersion int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM items WHERE owner_id = $1`, item.OwnerID).Scan(&nextVersion)
	if err != nil {
		return fmt.Errorf("postgres: next version: %w", err)
	}

	var tag pgconn.CommandTag
	if expectedVersion != nil {
		const q = `
			UPDATE items
			SET encrypted_data = $1,
			    metadata = $2,
			    type = $3,
			    version = $4,
			    updated_at = $5,
			    deleted = FALSE
			WHERE id = $6 AND owner_id = $7 AND version = $8 AND deleted = FALSE`
		tag, err = tx.Exec(ctx, q,
			sealed, metaJSON, int16(item.Type), nextVersion, item.UpdatedAt,
			item.ID, item.OwnerID, *expectedVersion,
		)
	} else {
		const q = `
			UPDATE items
			SET encrypted_data = $1,
			    metadata = $2,
			    type = $3,
			    version = $4,
			    updated_at = $5,
			    deleted = FALSE
			WHERE id = $6 AND owner_id = $7 AND deleted = FALSE`
		tag, err = tx.Exec(ctx, q,
			sealed, metaJSON, int16(item.Type), nextVersion, item.UpdatedAt,
			item.ID, item.OwnerID,
		)
	}
	if err != nil {
		return fmt.Errorf("postgres: update item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		exists, err := r.rowExists(ctx, tx, item.OwnerID, item.ID)
		if err != nil {
			return err
		}
		if !exists {
			return storage.ErrNotFound
		}
		if expectedVersion != nil {
			return storage.ErrVersionMismatch
		}
		return storage.ErrNotFound
	}
	item.Version = nextVersion
	item.Deleted = false
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// SoftDelete marks an item deleted and bumps its version for sync.
func (r *ItemRepository) SoftDelete(ctx context.Context, ownerID, id string) (*model.Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var nextVersion int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM items WHERE owner_id = $1`, ownerID).Scan(&nextVersion)
	if err != nil {
		return nil, fmt.Errorf("postgres: next version: %w", err)
	}
	now := time.Now().UTC()

	const q = `
		UPDATE items
		SET deleted = TRUE, version = $1, updated_at = $2
		WHERE id = $3 AND owner_id = $4 AND deleted = FALSE
		RETURNING id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted`
	item, err := r.scanItem(ctx, tx.QueryRow(ctx, q, nextVersion, now, id, ownerID))
	if errors.Is(err, storage.ErrNotFound) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: soft delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: commit: %w", err)
	}
	return item, nil
}

// GetByID returns an item owned by ownerID.
func (r *ItemRepository) GetByID(ctx context.Context, ownerID, id string) (*model.Item, error) {
	const q = `
		SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted
		FROM items WHERE id = $1 AND owner_id = $2`
	return r.scanItem(ctx, r.pool.QueryRow(ctx, q, id, ownerID))
}

// List returns items for an owner with optional type/deleted filters.
func (r *ItemRepository) List(ctx context.Context, ownerID string, filter storage.ListItemsFilter) ([]model.Item, error) {
	q := `
		SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted
		FROM items
		WHERE owner_id = $1`
	args := []any{ownerID}
	if !filter.IncludeDeleted {
		q += ` AND deleted = FALSE`
	}
	if filter.Type != nil {
		q += ` AND type = $2`
		args = append(args, int16(*filter.Type))
	}
	q += ` ORDER BY updated_at DESC, version DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list items: %w", err)
	}
	defer rows.Close()
	return r.collectItems(ctx, rows)
}

// ListChanged returns items with version > SinceVersion and/or updated_at > SinceTimestamp.
func (r *ItemRepository) ListChanged(ctx context.Context, ownerID string, filter storage.SyncFilter) ([]model.Item, error) {
	q := `
		SELECT id, owner_id, type, encrypted_data, metadata, version, updated_at, deleted
		FROM items
		WHERE owner_id = $1`
	args := []any{ownerID}

	if filter.SinceVersion > 0 {
		args = append(args, filter.SinceVersion)
		q += fmt.Sprintf(` AND version > $%d`, len(args))
	}
	if !filter.SinceTimestamp.IsZero() {
		args = append(args, filter.SinceTimestamp.UTC())
		q += fmt.Sprintf(` AND updated_at > $%d`, len(args))
	}
	q += ` ORDER BY version ASC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list changed: %w", err)
	}
	defer rows.Close()
	return r.collectItems(ctx, rows)
}

// MaxVersion returns the highest item version for an owner (0 if none).
func (r *ItemRepository) MaxVersion(ctx context.Context, ownerID string) (int64, error) {
	var version int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM items WHERE owner_id = $1`, ownerID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("postgres: max version: %w", err)
	}
	return version, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *ItemRepository) scanItem(ctx context.Context, row rowScanner) (*model.Item, error) {
	_ = ctx
	var (
		item     model.Item
		typ      int16
		sealed   []byte
		metaJSON []byte
	)
	err := row.Scan(
		&item.ID, &item.OwnerID, &typ, &sealed, &metaJSON,
		&item.Version, &item.UpdatedAt, &item.Deleted,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item.Type = model.DataType(typ)
	if r.cipher == nil {
		return nil, fmt.Errorf("postgres: decrypt item: cipher is nil")
	}
	plain, err := r.cipher.Decrypt(sealed)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt item: %w", err)
	}
	item.EncryptedData = plain
	item.Metadata, err = unmarshalMetadata(metaJSON)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ItemRepository) collectItems(ctx context.Context, rows pgx.Rows) ([]model.Item, error) {
	out := make([]model.Item, 0)
	for rows.Next() {
		item, err := r.scanItem(ctx, rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan item: %w", err)
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: rows: %w", err)
	}
	return out, nil
}

func (r *ItemRepository) rowExists(ctx context.Context, tx pgx.Tx, ownerID, id string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 AND owner_id = $2)`,
		id, ownerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: exists: %w", err)
	}
	return exists, nil
}

func marshalMetadata(m model.Metadata) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("postgres: marshal metadata: %w", err)
	}
	return b, nil
}

func unmarshalMetadata(b []byte) (model.Metadata, error) {
	if len(b) == 0 || string(b) == "null" {
		return model.Metadata{}, nil
	}
	var m model.Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("postgres: unmarshal metadata: %w", err)
	}
	if m == nil {
		m = model.Metadata{}
	}
	return m, nil
}

// Ensure interface compliance.
var _ storage.ItemRepository = (*ItemRepository)(nil)
