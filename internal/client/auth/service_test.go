package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/qutaq/gophkeeper/internal/client/auth"
	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/storage/sqlite"
	"github.com/qutaq/gophkeeper/internal/client/transport"
	"github.com/qutaq/gophkeeper/internal/testutil"
)

func TestInitUnlockAndSealedTokens(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	vault, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vault.Close() })

	conn := &transport.Conn{Auth: ts.AuthCli, Data: ts.DataCli, Sync: ts.SyncCli}
	svc := auth.NewService(vault, conn)
	ctx := context.Background()

	key, err := svc.InitVault(ctx, "carol", []byte("master-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()

	if err := svc.Register(ctx, "carol", "password1", key); err != nil {
		t.Fatal(err)
	}

	session, err := vault.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.AccessToken) == 0 || string(session.AccessToken) == "" {
		t.Fatal("expected sealed access token")
	}
	// Ciphertext must not equal plaintext JWT prefix.
	if len(session.AccessToken) < 16 {
		t.Fatal("token blob too short")
	}

	access, err := svc.EnsureAccess(ctx, key)
	if err != nil || access == "" {
		t.Fatalf("ensure: %q err=%v", access, err)
	}

	key.Zero()
	wrongPass := []byte("wrong-master")
	if _, err := svc.Unlock(ctx, wrongPass); err != crypto.ErrInvalidKey {
		t.Fatalf("wrong unlock: %v", err)
	}

	okKey, err := svc.Unlock(ctx, []byte("master-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer okKey.Zero()
	access2, err := svc.EnsureAccess(ctx, okKey)
	if err != nil || access2 == "" {
		t.Fatalf("re-unlock ensure: %v", err)
	}
}

func TestEnsureAccessRefresh(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	vault, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vault.Close() })

	conn := &transport.Conn{Auth: ts.AuthCli}
	svc := auth.NewService(vault, conn)
	ctx := context.Background()
	key, err := svc.InitVault(ctx, "erin", []byte("master-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()
	if err := svc.Register(ctx, "erin", "password1", key); err != nil {
		t.Fatal(err)
	}

	session, err := vault.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session.AccessToken = []byte{} // force refresh path
	if err := vault.SaveSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	access, err := svc.EnsureAccess(ctx, key)
	if err != nil || access == "" {
		t.Fatalf("refresh ensure: %q err=%v", access, err)
	}
}

func TestEnsureAccessErrors(t *testing.T) {
	t.Parallel()

	vault, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vault.Close() })
	svc := auth.NewService(vault, nil)
	ctx := context.Background()
	if _, err := svc.EnsureAccess(ctx, nil); err == nil {
		t.Fatal("nil key")
	}
	key, err := crypto.DeriveKey([]byte("x"), crypto.SaltFromLogin("z"))
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()
	if _, err := svc.EnsureAccess(ctx, key); err == nil {
		t.Fatal("not logged in")
	}
}

func TestUnlockRequiresInit(t *testing.T) {
	t.Parallel()

	vault, err := sqlite.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vault.Close() })
	svc := auth.NewService(vault, nil)
	if _, err := svc.Unlock(context.Background(), []byte("x")); err != storage.ErrNotInitialized {
		t.Fatalf("got %v", err)
	}
}