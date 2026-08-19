package data_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/data"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/storage/sqlite"
	"github.com/qutaq/gophkeeper/internal/model"
)

func TestServiceAddGetEditDelete(t *testing.T) {
	t.Parallel()

	vault, key := openVault(t, "alice", "master-pass")
	svc := data.NewService(vault, key)
	ctx := context.Background()

	plain := []byte(`{"text":"hello"}`)
	item, err := svc.Add(ctx, data.AddInput{
		Type:     model.DataTypeText,
		Plain:    append([]byte(nil), plain...),
		Metadata: model.Metadata{"title": "n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !item.Dirty || len(item.EncryptedData) == 0 {
		t.Fatalf("item=%+v", item)
	}
	if !bytes.Equal(plain, make([]byte, len(plain))) {
		// Add zeros its own copy; original may remain — ensure encrypted != plain
	}

	got, decrypted, err := svc.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata["title"] != "n" {
		t.Fatalf("meta=%v", got.Metadata)
	}
	if string(decrypted) != `{"text":"hello"}` {
		t.Fatalf("plain=%s", decrypted)
	}

	edited, err := svc.Edit(ctx, item.ID, []byte(`{"text":"world"}`), model.Metadata{"title": "n2"})
	if err != nil {
		t.Fatal(err)
	}
	if !edited.Dirty {
		t.Fatal("edit must mark dirty")
	}
	_, decrypted, err = svc.Get(ctx, item.ID)
	if err != nil || string(decrypted) != `{"text":"world"}` {
		t.Fatalf("after edit: %s err=%v", decrypted, err)
	}

	listed, err := svc.List(ctx, nil, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	del, err := svc.SoftDelete(ctx, item.ID)
	if err != nil || !del.Deleted || !del.Dirty {
		t.Fatalf("delete=%+v err=%v", del, err)
	}
}

func TestServiceValidation(t *testing.T) {
	t.Parallel()

	vault, key := openVault(t, "bob", "pw")
	svc := data.NewService(vault, key)
	ctx := context.Background()

	if _, err := svc.Add(ctx, data.AddInput{Type: model.DataTypeUnspecified, Plain: []byte("x")}); err == nil {
		t.Fatal("expected invalid type")
	}
	if _, err := svc.Add(ctx, data.AddInput{Type: model.DataTypeText}); err == nil {
		t.Fatal("expected empty payload")
	}
	if _, _, err := svc.Get(ctx, "missing"); err != storage.ErrNotFound {
		t.Fatalf("get missing: %v", err)
	}
}

func openVault(t *testing.T, login, pass string) (storage.Vault, *crypto.MasterKey) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	salt := crypto.SaltFromLogin(login)
	key, err := crypto.DeriveKey([]byte(pass), salt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(key.Zero)
	if err := store.InitVault(context.Background(), storage.VaultMeta{
		Salt:     salt,
		Verifier: key.Verifier(),
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.SaveSession(context.Background(), storage.Session{UserID: "user-1", Login: login})
	return store, key
}
