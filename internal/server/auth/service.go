package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

const (
	minLoginLen    = 3
	maxLoginLen    = 64
	minPasswordLen = 8
	maxPasswordLen = 128
)

// Service provides authentication use-cases.
type Service struct {
	users   storage.UserRepository
	refresh storage.RefreshTokenRepository
	tokens  *TokenManager
	now     func() time.Time
}

// NewService constructs an auth Service.
func NewService(users storage.UserRepository, refresh storage.RefreshTokenRepository, tokens *TokenManager) *Service {
	return &Service{
		users:   users,
		refresh: refresh,
		tokens:  tokens,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// TokenPair is an access + refresh token bundle.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// Register creates a new user after validating input.
func (s *Service) Register(ctx context.Context, login, password string) (*model.User, error) {
	login = strings.TrimSpace(login)
	if err := validateCredentials(login, password); err != nil {
		return nil, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &model.User{
		Login:        login,
		PasswordHash: hash,
	}
	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return nil, ErrLoginTaken
		}
		return nil, fmt.Errorf("auth: register: %w", err)
	}
	user.PasswordHash = ""
	return user, nil
}

// Login verifies credentials and issues a token pair.
func (s *Service) Login(ctx context.Context, login, password string) (*model.User, *TokenPair, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return nil, nil, ErrInvalidCredentials
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, fmt.Errorf("auth: login: %w", err)
	}

	ok, err := VerifyPassword(user.PasswordHash, password)
	if err != nil || !ok {
		return nil, nil, ErrInvalidCredentials
	}

	pair, err := s.issuePair(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	user.PasswordHash = ""
	return user, pair, nil
}

// Refresh rotates a refresh token and returns a new pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	stored, err := s.lookupRefresh(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if err := s.refresh.Revoke(ctx, stored.TokenHash); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("auth: revoke old refresh: %w", err)
	}
	return s.issuePair(ctx, stored.UserID)
}

// Logout revokes the given refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashRefreshToken(refreshToken)
	err := s.refresh.Revoke(ctx, hash)
	if errors.Is(err, storage.ErrNotFound) {
		return nil // already gone — idempotent
	}
	return err
}

// ParseAccessToken validates an access JWT and returns the user id.
func (s *Service) ParseAccessToken(accessToken string) (string, error) {
	claims, err := s.tokens.ParseAndValidate(accessToken, TokenTypeAccess)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}

func (s *Service) issuePair(ctx context.Context, userID string) (*TokenPair, error) {
	access, _, err := s.tokens.IssueAccess(userID)
	if err != nil {
		return nil, err
	}
	refreshRaw, err := NewOpaqueRefreshToken()
	if err != nil {
		return nil, err
	}
	expires := s.now().Add(s.tokens.RefreshTTL())
	rec := &storage.RefreshToken{
		UserID:    userID,
		TokenHash: hashRefreshToken(refreshRaw),
		ExpiresAt: expires,
	}
	if err := s.refresh.Create(ctx, rec); err != nil {
		return nil, fmt.Errorf("auth: store refresh: %w", err)
	}
	return &TokenPair{AccessToken: access, RefreshToken: refreshRaw}, nil
}

func (s *Service) lookupRefresh(ctx context.Context, refreshToken string) (*storage.RefreshToken, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, ErrInvalidToken
	}
	stored, err := s.refresh.GetByHash(ctx, hashRefreshToken(refreshToken))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("auth: lookup refresh: %w", err)
	}
	if stored.RevokedAt != nil || !stored.ExpiresAt.After(s.now()) {
		return nil, ErrInvalidToken
	}
	return stored, nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func validateCredentials(login, password string) error {
	loginLen := utf8.RuneCountInString(login)
	if loginLen < minLoginLen || loginLen > maxLoginLen {
		return fmt.Errorf("%w: login length must be %d-%d", ErrInvalidInput, minLoginLen, maxLoginLen)
	}
	for _, r := range login {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("%w: login has invalid characters", ErrInvalidInput)
	}
	passLen := utf8.RuneCountInString(password)
	if passLen < minPasswordLen || passLen > maxPasswordLen {
		return fmt.Errorf("%w: password length must be %d-%d", ErrInvalidInput, minPasswordLen, maxPasswordLen)
	}
	return nil
}
