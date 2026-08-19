package auth

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "correct-horse")
	if err != nil || !ok {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("wrong password must fail")
	}
}

func TestKeyRingRotation(t *testing.T) {
	t.Parallel()

	k1 := SigningKey{ID: "v1", Key: bytes.Repeat([]byte{1}, 32)}
	k2 := SigningKey{ID: "v2", Key: bytes.Repeat([]byte{2}, 32)}
	ring, err := NewKeyRing(k1)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := NewTokenManager(TokenManagerConfig{Keys: ring, AccessTTL: time.Minute, RefreshTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}

	oldAccess, _, err := tm.IssueAccess("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ring.Rotate(k2); err != nil {
		t.Fatal(err)
	}
	if ring.CurrentKID() != "v2" {
		t.Fatalf("kid=%s", ring.CurrentKID())
	}

	// Old token still verifies via previous key.
	claims, err := tm.ParseAndValidate(oldAccess, TokenTypeAccess)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("uid=%s", claims.UserID)
	}

	newAccess, _, err := tm.IssueAccess("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.ParseAndValidate(newAccess, TokenTypeAccess); err != nil {
		t.Fatal(err)
	}
}

func TestParseKeyRingEnv(t *testing.T) {
	t.Parallel()

	k1 := hex.EncodeToString(bytes.Repeat([]byte{3}, 32))
	k2 := hex.EncodeToString(bytes.Repeat([]byte{4}, 32))
	ring, err := ParseKeyRingEnv("v1:"+k1+",v2:"+k2, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if ring.CurrentKID() != "v2" {
		t.Fatalf("kid=%s", ring.CurrentKID())
	}
}

func TestServiceRegisterLoginRefreshLogout(t *testing.T) {
	t.Parallel()

	users := newMemUsers()
	refresh := newMemRefresh()
	key := SigningKey{ID: "v1", Key: bytes.Repeat([]byte{9}, 32)}
	ring, err := NewKeyRing(key)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := NewTokenManager(TokenManagerConfig{Keys: ring, AccessTTL: time.Minute, RefreshTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(users, refresh, tm)
	ctx := context.Background()

	if _, err := svc.Register(ctx, "ab", "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}

	user, err := svc.Register(ctx, "alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "alice", "password123"); !errors.Is(err, ErrLoginTaken) {
		t.Fatalf("want ErrLoginTaken, got %v", err)
	}

	got, pair, err := svc.Login(ctx, "alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("unexpected login result")
	}
	if _, _, err := svc.Login(ctx, "alice", "nope"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}

	uid, err := svc.ParseAccessToken(pair.AccessToken)
	if err != nil || uid != user.ID {
		t.Fatalf("parse access: uid=%s err=%v", uid, err)
	}

	next, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("old refresh must be revoked, got %v", err)
	}

	if err := svc.Logout(ctx, next.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, next.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("logged out refresh must fail, got %v", err)
	}
}

type memUsers struct {
	mu      sync.Mutex
	byID    map[string]*model.User
	byLogin map[string]*model.User
}

func newMemUsers() *memUsers {
	return &memUsers{byID: map[string]*model.User{}, byLogin: map[string]*model.User{}}
}

func (m *memUsers) Create(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byLogin[user.Login]; ok {
		return storage.ErrConflict
	}
	if user.ID == "" {
		user.ID = hex.EncodeToString([]byte(user.Login + "-id"))[:16]
	}
	cp := *user
	m.byID[user.ID] = &cp
	m.byLogin[user.Login] = &cp
	return nil
}

func (m *memUsers) GetByID(_ context.Context, id string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *memUsers) GetByLogin(_ context.Context, login string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byLogin[login]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

type memRefresh struct {
	mu     sync.Mutex
	byHash map[string]*storage.RefreshToken
}

func newMemRefresh() *memRefresh {
	return &memRefresh{byHash: map[string]*storage.RefreshToken{}}
}

func (m *memRefresh) Create(_ context.Context, token *storage.RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byHash[token.TokenHash]; ok {
		return storage.ErrConflict
	}
	if token.ID == "" {
		token.ID = token.TokenHash[:16]
	}
	cp := *token
	m.byHash[token.TokenHash] = &cp
	return nil
}

func (m *memRefresh) GetByHash(_ context.Context, tokenHash string) (*storage.RefreshToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byHash[tokenHash]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *memRefresh) Revoke(_ context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byHash[tokenHash]
	if !ok || t.RevokedAt != nil {
		return storage.ErrNotFound
	}
	now := time.Now().UTC()
	t.RevokedAt = &now
	return nil
}

func (m *memRefresh) RevokeAllForUser(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, t := range m.byHash {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
		}
	}
	return nil
}
