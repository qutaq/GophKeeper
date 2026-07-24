package syncer_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/qutaq/gophkeeper/internal/client/auth"
	"github.com/qutaq/gophkeeper/internal/client/data"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/storage/sqlite"
	"github.com/qutaq/gophkeeper/internal/client/syncer"
	"github.com/qutaq/gophkeeper/internal/client/transport"
	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/testutil"
)

func TestBidirectionalSync(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	ctx := context.Background()

	vaultA := openClientVault(t, "dave")
	conn := &transport.Conn{Auth: ts.AuthCli, Data: ts.DataCli, Sync: ts.SyncCli}
	authA := auth.NewService(vaultA, conn)
	keyA, err := authA.InitVault(ctx, "dave", []byte("master-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer keyA.Zero()
	if err := authA.Register(ctx, "dave", "password1", keyA); err != nil {
		t.Fatal(err)
	}

	dataA := data.NewService(vaultA, keyA)
	item, err := dataA.Add(ctx, data.AddInput{
		Type:  model.DataTypeText,
		Plain: []byte(`{"text":"from-a"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := authA.EnsureAccess(ctx, keyA)
	if err != nil {
		t.Fatal(err)
	}
	syncA := syncer.NewService(vaultA, conn)
	res, err := syncA.Sync(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 {
		t.Fatalf("pushed=%d", res.Pushed)
	}

	vaultB := openClientVault(t, "dave-b")
	authB := auth.NewService(vaultB, conn)
	keyB, err := authB.InitVault(ctx, "dave", []byte("master-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer keyB.Zero()
	if err := authB.Login(ctx, "dave", "password1", "", keyB); err != nil {
		t.Fatal(err)
	}
	tokenB, err := authB.EnsureAccess(ctx, keyB)
	if err != nil {
		t.Fatal(err)
	}
	syncB := syncer.NewService(vaultB, conn)
	resB, err := syncB.Sync(ctx, tokenB)
	if err != nil {
		t.Fatal(err)
	}
	if resB.Pulled < 1 {
		t.Fatalf("pulled=%d", resB.Pulled)
	}

	dataB := data.NewService(vaultB, keyB)
	listed, err := dataB.List(ctx, nil, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	_, plain, err := dataB.Get(ctx, listed[0].ID)
	if err != nil || string(plain) != `{"text":"from-a"}` {
		t.Fatalf("plain=%s err=%v (local id was %s)", plain, err, item.ID)
	}
}

func TestSyncPushDeleteAndUpdate(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	ctx := context.Background()
	vault := openClientVault(t, "frank")
	conn := &transport.Conn{Auth: ts.AuthCli, Data: ts.DataCli, Sync: ts.SyncCli}
	authSvc := auth.NewService(vault, conn)
	key, err := authSvc.InitVault(ctx, "frank", []byte("master"))
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()
	if err := authSvc.Register(ctx, "frank", "password1", key); err != nil {
		t.Fatal(err)
	}
	dataSvc := data.NewService(vault, key)
	item, err := dataSvc.Add(ctx, data.AddInput{Type: model.DataTypeText, Plain: []byte(`{"text":"v1"}`)})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authSvc.EnsureAccess(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	syncSvc := syncer.NewService(vault, conn)
	if _, err := syncSvc.Sync(ctx, token); err != nil {
		t.Fatal(err)
	}

	listed, err := dataSvc.List(ctx, nil, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("after first sync list=%v err=%v (old id %s)", listed, err, item.ID)
	}
	if _, err := dataSvc.Edit(ctx, listed[0].ID, []byte(`{"text":"v2"}`), nil); err != nil {
		t.Fatal(err)
	}
	token, err = authSvc.EnsureAccess(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncSvc.Sync(ctx, token); err != nil {
		t.Fatal(err)
	}

	listed, err = dataSvc.List(ctx, nil, false)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if _, err := dataSvc.SoftDelete(ctx, listed[0].ID); err != nil {
		t.Fatal(err)
	}
	token, err = authSvc.EnsureAccess(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	res, err := syncSvc.Sync(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed < 1 {
		t.Fatalf("expected delete push, got %+v", res)
	}
}

func openClientVault(t *testing.T, name string) storage.Vault {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), name+"-vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
