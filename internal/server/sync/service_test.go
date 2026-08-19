package syncsvc_test

import (
	"context"
	"testing"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/data"
	syncsvc "github.com/qutaq/gophkeeper/internal/server/sync"
	"github.com/qutaq/gophkeeper/internal/testutil"
)

func TestPullIncludesDeletesAndScopesByOwner(t *testing.T) {
	t.Parallel()

	repo := testutil.NewMemItems()
	dataSvc := data.NewService(repo)
	syncService := syncsvc.NewService(repo)
	ctx := context.Background()

	aliceItem, err := dataSvc.Add(ctx, "alice", data.AddInput{
		Type:          model.DataTypeCredentials,
		EncryptedData: []byte("alice-secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataSvc.Add(ctx, "bob", data.AddInput{
		Type:          model.DataTypeText,
		EncryptedData: []byte("bob-secret"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dataSvc.Delete(ctx, "alice", aliceItem.ID); err != nil {
		t.Fatal(err)
	}

	result, err := syncService.Pull(ctx, "alice", 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ServerVersion != 2 || len(result.Items) != 1 || !result.Items[0].Deleted {
		t.Fatalf("alice result=%+v", result)
	}
	if string(result.Items[0].EncryptedData) != "alice-secret" {
		t.Fatal("alice must not receive bob ciphertext")
	}

	bob, err := syncService.Pull(ctx, "bob", 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bob.Items) != 1 || bob.Items[0].Deleted || bob.ServerVersion != 1 {
		t.Fatalf("bob result=%+v", bob)
	}

	empty, err := syncService.Pull(ctx, "alice", 2, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Items) != 0 || empty.ServerVersion != 2 {
		t.Fatalf("empty=%+v", empty)
	}
}

func TestPullRequiresOwner(t *testing.T) {
	t.Parallel()

	svc := syncsvc.NewService(testutil.NewMemItems())
	if _, err := svc.Pull(context.Background(), "", 0, time.Time{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPullSinceTimestamp(t *testing.T) {
	t.Parallel()

	repo := testutil.NewMemItems()
	dataSvc := data.NewService(repo)
	syncService := syncsvc.NewService(repo)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	item, err := dataSvc.Add(ctx, "u", data.AddInput{
		Type:          model.DataTypeText,
		EncryptedData: []byte("x"),
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := syncService.Pull(ctx, "u", 0, before)
	if err != nil || len(all.Items) != 1 {
		t.Fatalf("since before: %+v err=%v", all, err)
	}

	after := item.UpdatedAt.Add(time.Second)
	none, err := syncService.Pull(ctx, "u", 0, after)
	if err != nil || len(none.Items) != 0 {
		t.Fatalf("since after: %+v err=%v", none, err)
	}
}
