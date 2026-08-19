package data_test

import (
	"context"
	"errors"
	"testing"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/data"
	"github.com/qutaq/gophkeeper/internal/server/storage"
	"github.com/qutaq/gophkeeper/internal/testutil"
)

func TestDataServiceCRUDAndConflicts(t *testing.T) {
	t.Parallel()

	repo := testutil.NewMemItems()
	svc := data.NewService(repo)
	ctx := context.Background()
	owner := "owner-1"

	item, err := svc.Add(ctx, owner, data.AddInput{
		Type:          model.DataTypeText,
		EncryptedData: []byte("cipher-1"),
		Metadata:      model.Metadata{"note": "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Version != 1 {
		t.Fatalf("version=%d", item.Version)
	}

	got, err := svc.Get(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.EncryptedData) != "cipher-1" {
		t.Fatalf("payload=%q", got.EncryptedData)
	}

	updated, err := svc.Update(ctx, owner, data.UpdateInput{
		ID:              item.ID,
		EncryptedData:   []byte("cipher-2"),
		Metadata:        model.Metadata{"note": "b"},
		ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("version=%d", updated.Version)
	}

	_, err = svc.Update(ctx, owner, data.UpdateInput{
		ID:              item.ID,
		EncryptedData:   []byte("stale"),
		ExpectedVersion: 1,
	})
	if !errors.Is(err, data.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}

	lww, err := svc.Update(ctx, owner, data.UpdateInput{
		ID:              item.ID,
		EncryptedData:   []byte("cipher-lww"),
		ExpectedVersion: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(lww.EncryptedData) != "cipher-lww" || lww.Version != 3 {
		t.Fatalf("lww=%+v", lww)
	}

	deleted, err := svc.Delete(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Deleted || deleted.Version != 4 {
		t.Fatalf("deleted=%+v", deleted)
	}

	listed, err := svc.List(ctx, owner, storage.ListItemsFilter{IncludeDeleted: true})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
}

func TestForeignOwnerCannotAccessItems(t *testing.T) {
	t.Parallel()

	repo := testutil.NewMemItems()
	svc := data.NewService(repo)
	ctx := context.Background()

	item, err := svc.Add(ctx, "alice", data.AddInput{
		Type:          model.DataTypeText,
		EncryptedData: []byte("secret"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Get(ctx, "bob", item.ID); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("get: want ErrNotFound, got %v", err)
	}
	if _, err := svc.Update(ctx, "bob", data.UpdateInput{
		ID:              item.ID,
		EncryptedData:   []byte("hacked"),
		ExpectedVersion: 1,
	}); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("update: want ErrNotFound, got %v", err)
	}
	if _, err := svc.Delete(ctx, "bob", item.ID); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("delete: want ErrNotFound, got %v", err)
	}

	listed, err := svc.List(ctx, "bob", storage.ListItemsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("bob must not see alice items: %v", listed)
	}

	// Alice still owns the record.
	got, err := svc.Get(ctx, "alice", item.ID)
	if err != nil || string(got.EncryptedData) != "secret" {
		t.Fatalf("alice item broken: %+v err=%v", got, err)
	}
}

func TestDataServiceValidation(t *testing.T) {
	t.Parallel()

	svc := data.NewService(testutil.NewMemItems())
	ctx := context.Background()

	if _, err := svc.Add(ctx, "", data.AddInput{Type: model.DataTypeText, EncryptedData: []byte("x")}); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("add empty owner: %v", err)
	}
	if _, err := svc.Add(ctx, "o", data.AddInput{Type: model.DataTypeUnspecified, EncryptedData: []byte("x")}); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("add bad type: %v", err)
	}
	if _, err := svc.Add(ctx, "o", data.AddInput{Type: model.DataTypeText}); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("add empty payload: %v", err)
	}
	if _, err := svc.Get(ctx, "", "id"); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("get validation: %v", err)
	}
	if _, err := svc.Get(ctx, "o", "missing"); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if _, err := svc.Delete(ctx, "o", ""); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("delete validation: %v", err)
	}
	if _, err := svc.List(ctx, "", storage.ListItemsFilter{}); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("list validation: %v", err)
	}
	bad := model.DataType(99)
	if _, err := svc.List(ctx, "o", storage.ListItemsFilter{Type: &bad}); !errors.Is(err, data.ErrInvalidInput) {
		t.Fatalf("list bad type: %v", err)
	}

	item, err := svc.Add(ctx, "o", data.AddInput{Type: model.DataTypeText, EncryptedData: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(ctx, "o", item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "o", data.UpdateInput{
		ID: item.ID, EncryptedData: []byte("y"), ExpectedVersion: 0,
	}); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("update deleted: %v", err)
	}
}
