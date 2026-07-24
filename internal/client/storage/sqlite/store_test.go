package sqlite_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/storage/sqlite"
	"github.com/qutaq/gophkeeper/internal/model"
)

func TestVaultCRUD(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if err := store.InitVault(ctx, storage.VaultMeta{Salt: []byte("0123456789abcdef"), Verifier: "v"}); err != nil {
		t.Fatal(err)
	}
	if err := store.InitVault(ctx, storage.VaultMeta{Salt: []byte("0123456789abcdef"), Verifier: "v"}); err != storage.ErrAlreadyInitialized {
		t.Fatalf("second init: %v", err)
	}
	meta, err := store.GetVaultMeta(ctx)
	if err != nil || meta.Verifier != "v" {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}

	item := storage.LocalItem{
		Item: model.Item{
			ID:            "id-1",
			Type:          model.DataTypeText,
			EncryptedData: []byte("cipher"),
			Metadata:      model.Metadata{"k": "v"},
			Version:       1,
		},
		Dirty: true,
	}
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetItem(ctx, "id-1")
	if err != nil || !got.Dirty || got.Metadata["k"] != "v" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	dirty, err := store.ListDirty(ctx)
	if err != nil || len(dirty) != 1 {
		t.Fatalf("dirty=%v err=%v", dirty, err)
	}

	typ := model.DataTypeText
	listed, err := store.ListItems(ctx, &typ, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	if err := store.SetServerVersion(ctx, 42); err != nil {
		t.Fatal(err)
	}
	ver, err := store.GetServerVersion(ctx)
	if err != nil || ver != 42 {
		t.Fatalf("ver=%d err=%v", ver, err)
	}

	if err := store.DeleteItemHard(ctx, "id-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetItem(ctx, "id-1"); err != storage.ErrNotFound {
		t.Fatalf("after hard delete: %v", err)
	}
	if err := store.DeleteItemHard(ctx, "missing"); err != storage.ErrNotFound {
		t.Fatalf("hard delete missing: %v", err)
	}
}

func TestSessionPersistence(t *testing.T) {
	t.Parallel()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	session := storage.Session{
		UserID:        "u1",
		Login:         "alice",
		AccessToken:   []byte("enc-access"),
		RefreshToken:  []byte("enc-refresh"),
		ServerVersion: 7,
	}
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "u1" || got.Login != "alice" || got.ServerVersion != 7 {
		t.Fatalf("got=%+v", got)
	}
	if !bytes.Equal(got.AccessToken, []byte("enc-access")) || !bytes.Equal(got.RefreshToken, []byte("enc-refresh")) {
		t.Fatalf("tokens=%q/%q", got.AccessToken, got.RefreshToken)
	}

	if err := store.ClearSession(ctx); err != nil {
		t.Fatal(err)
	}
	cleared, err := store.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.AccessToken) != 0 || len(cleared.RefreshToken) != 0 {
		t.Fatalf("tokens not cleared: %+v", cleared)
	}
	if cleared.UserID != "u1" {
		t.Fatalf("user id should remain: %+v", cleared)
	}
}

func TestGetVaultMetaNotInitialized(t *testing.T) {
	t.Parallel()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.GetVaultMeta(context.Background()); err != storage.ErrNotInitialized {
		t.Fatalf("got %v", err)
	}
}
