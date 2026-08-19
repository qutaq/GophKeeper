package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
	"github.com/qutaq/gophkeeper/internal/server/storage/postgres"
	"github.com/qutaq/gophkeeper/pkg/crypto/atrest"
)

func TestStoreIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.MigrateUp(postgres.ToMigrateURL(dsn), migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	key := make([]byte, atrest.KeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, err := atrest.NewCipherFromProvider(ctx, atrest.StaticKey{Key: key})
	if err != nil {
		t.Fatal(err)
	}

	store, err := postgres.Open(ctx, dsn, cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	login := "phase2_" + time.Now().UTC().Format("150405.000")
	user := &model.User{Login: login, PasswordHash: "hash"}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	gotUser, err := store.Users.GetByLogin(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser.ID != user.ID {
		t.Fatalf("user id mismatch")
	}

	item := &model.Item{
		OwnerID:       user.ID,
		Type:          model.DataTypeCredentials,
		EncryptedData: []byte("e2e-ciphertext"),
		Metadata:      model.Metadata{"site": "example.com"},
	}
	if err := store.Items.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if item.Version != 1 {
		t.Fatalf("version = %d, want 1", item.Version)
	}

	got, err := store.Items.GetByID(ctx, user.ID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.EncryptedData) != "e2e-ciphertext" {
		t.Fatalf("payload = %q", got.EncryptedData)
	}
	if got.Metadata["site"] != "example.com" {
		t.Fatalf("metadata = %#v", got.Metadata)
	}

	got.EncryptedData = []byte("updated")
	got.Metadata["site"] = "updated.com"
	if err := store.Items.Update(ctx, got, 1); err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 {
		t.Fatalf("version = %d, want 2", got.Version)
	}

	if err := store.Items.Update(ctx, got, 1); err != storage.ErrVersionMismatch {
		t.Fatalf("want ErrVersionMismatch, got %v", err)
	}

	deleted, err := store.Items.SoftDelete(ctx, user.ID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Deleted || deleted.Version != 3 {
		t.Fatalf("deleted=%v version=%d", deleted.Deleted, deleted.Version)
	}

	changed, err := store.Items.ListChanged(ctx, user.ID, storage.SyncFilter{SinceVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || !changed[0].Deleted {
		t.Fatalf("sync items = %#v", changed)
	}
}

func TestForeignOwnerIsolation(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.MigrateUp(postgres.ToMigrateURL(dsn), migrationsDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	key := make([]byte, atrest.KeySize)
	for i := range key {
		key[i] = byte(i + 3)
	}
	cipher, err := atrest.NewCipherFromProvider(ctx, atrest.StaticKey{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, dsn, cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	suffix := time.Now().UTC().Format("150405.000")
	alice := &model.User{Login: "alice_" + suffix, PasswordHash: "h"}
	bob := &model.User{Login: "bob_" + suffix, PasswordHash: "h"}
	if err := store.Users.Create(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if err := store.Users.Create(ctx, bob); err != nil {
		t.Fatal(err)
	}

	item := &model.Item{
		OwnerID:       alice.ID,
		Type:          model.DataTypeText,
		EncryptedData: []byte("alice-cipher"),
	}
	if err := store.Items.Create(ctx, item); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Items.GetByID(ctx, bob.ID, item.ID); err != storage.ErrNotFound {
		t.Fatalf("bob get: %v", err)
	}
	listed, err := store.Items.List(ctx, bob.ID, storage.ListItemsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("bob list leaked: %#v", listed)
	}
	changed, err := store.Items.ListChanged(ctx, bob.ID, storage.SyncFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("bob sync leaked: %#v", changed)
	}
}
