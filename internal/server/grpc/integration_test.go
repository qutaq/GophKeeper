package grpcserver_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	"github.com/qutaq/gophkeeper/internal/testutil"
)

func TestGRPCAuthDataSyncFlow(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	ctx := context.Background()

	_, err := ts.AuthCli.Register(ctx, gophkeeperv1.RegisterRequest_builder{
		Login:    "alice",
		Password: "password1",
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	login, err := ts.AuthCli.Login(ctx, gophkeeperv1.LoginRequest_builder{
		Login:    "alice",
		Password: "password1",
	}.Build())
	if err != nil {
		t.Fatal(err)
	}

	authCtx := testutil.AuthCtx(ctx, login.GetAccessToken())
	added, err := ts.DataCli.AddItem(authCtx, gophkeeperv1.AddItemRequest_builder{
		Type:          gophkeeperv1.DataType_DATA_TYPE_TEXT,
		EncryptedData: []byte("e2e-cipher"),
		Metadata:      map[string]string{"k": "v"},
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	id := added.GetItem().GetId()

	got, err := ts.DataCli.GetItem(authCtx, gophkeeperv1.GetItemRequest_builder{Id: id}.Build())
	if err != nil || string(got.GetItem().GetEncryptedData()) != "e2e-cipher" {
		t.Fatalf("get=%v err=%v", got, err)
	}

	updated, err := ts.DataCli.UpdateItem(authCtx, gophkeeperv1.UpdateItemRequest_builder{
		Id: id, EncryptedData: []byte("e2e-cipher-2"), ExpectedVersion: 1,
		Metadata: map[string]string{"k": "v2"},
	}.Build())
	if err != nil || updated.GetItem().GetVersion() != 2 {
		t.Fatalf("update=%v err=%v", updated, err)
	}

	listed, err := ts.DataCli.ListItems(authCtx, gophkeeperv1.ListItemsRequest_builder{}.Build())
	if err != nil || len(listed.GetItems()) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	deleted, err := ts.DataCli.DeleteItem(authCtx, gophkeeperv1.DeleteItemRequest_builder{Id: id}.Build())
	if err != nil || !deleted.GetItem().GetDeleted() {
		t.Fatalf("delete=%v err=%v", deleted, err)
	}

	syncResp, err := ts.SyncCli.Sync(authCtx, gophkeeperv1.SyncRequest_builder{SinceVersion: 0}.Build())
	if err != nil || len(syncResp.GetItems()) != 1 || !syncResp.GetItems()[0].GetDeleted() {
		t.Fatalf("sync=%v err=%v", syncResp, err)
	}
}

func TestGRPCForeignOwnerDenied(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	ctx := context.Background()

	if _, err := ts.AuthCli.Register(ctx, gophkeeperv1.RegisterRequest_builder{Login: "alice", Password: "password1"}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.AuthCli.Register(ctx, gophkeeperv1.RegisterRequest_builder{Login: "bob", Password: "password1"}.Build()); err != nil {
		t.Fatal(err)
	}
	aliceLogin, err := ts.AuthCli.Login(ctx, gophkeeperv1.LoginRequest_builder{Login: "alice", Password: "password1"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	bobLogin, err := ts.AuthCli.Login(ctx, gophkeeperv1.LoginRequest_builder{Login: "bob", Password: "password1"}.Build())
	if err != nil {
		t.Fatal(err)
	}

	aliceCtx := testutil.AuthCtx(ctx, aliceLogin.GetAccessToken())
	bobCtx := testutil.AuthCtx(ctx, bobLogin.GetAccessToken())

	added, err := ts.DataCli.AddItem(aliceCtx, gophkeeperv1.AddItemRequest_builder{
		Type:          gophkeeperv1.DataType_DATA_TYPE_TEXT,
		EncryptedData: []byte("alice-only"),
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	id := added.GetItem().GetId()

	_, err = ts.DataCli.GetItem(bobCtx, gophkeeperv1.GetItemRequest_builder{Id: id}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("bob get: want NotFound, got %v", err)
	}
	_, err = ts.DataCli.UpdateItem(bobCtx, gophkeeperv1.UpdateItemRequest_builder{
		Id: id, EncryptedData: []byte("hack"), ExpectedVersion: 1,
	}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("bob update: want NotFound, got %v", err)
	}
	_, err = ts.DataCli.DeleteItem(bobCtx, gophkeeperv1.DeleteItemRequest_builder{Id: id}.Build())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("bob delete: want NotFound, got %v", err)
	}

	list, err := ts.DataCli.ListItems(bobCtx, gophkeeperv1.ListItemsRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetItems()) != 0 {
		t.Fatalf("bob listed foreign items: %v", list.GetItems())
	}

	syncResp, err := ts.SyncCli.Sync(bobCtx, gophkeeperv1.SyncRequest_builder{}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(syncResp.GetItems()) != 0 {
		t.Fatalf("bob synced foreign items: %v", syncResp.GetItems())
	}
}

func TestGRPCRequiresAuth(t *testing.T) {
	t.Parallel()

	ts := testutil.StartTestServer(t)
	_, err := ts.DataCli.ListItems(context.Background(), gophkeeperv1.ListItemsRequest_builder{}.Build())
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}
