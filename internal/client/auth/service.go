// Package auth wraps remote authentication and local session persistence.
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/transport"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	"github.com/qutaq/gophkeeper/pkg/secure"
)

// refreshSkew refreshes slightly before exp so in-flight RPCs are less likely to
// hit Unauthenticated at the exact expiry boundary.
const refreshSkew = 30 * time.Second

// Service handles register/login against the server and local vault unlock.
type Service struct {
	vault storage.Vault
	conn  *transport.Conn
}

// NewService constructs an auth Service.
func NewService(vault storage.Vault, conn *transport.Conn) *Service {
	return &Service{vault: vault, conn: conn}
}

// InitVault creates local KDF material from master password and login.
// masterPassword is consumed (zeroed) by DeriveKey.
func (s *Service) InitVault(ctx context.Context, login string, masterPassword []byte) (*crypto.MasterKey, error) {
	salt := crypto.SaltFromLogin(login)
	key, err := crypto.DeriveKey(masterPassword, salt)
	if err != nil {
		return nil, err
	}
	if err := s.vault.InitVault(ctx, storage.VaultMeta{
		Salt:     salt,
		Verifier: key.Verifier(),
	}); err != nil {
		key.Zero()
		return nil, err
	}
	return key, nil
}

// Unlock derives and verifies the master key.
// masterPassword is consumed (zeroed) by DeriveKey.
func (s *Service) Unlock(ctx context.Context, masterPassword []byte) (*crypto.MasterKey, error) {
	meta, err := s.vault.GetVaultMeta(ctx)
	if err != nil {
		return nil, err
	}
	key, err := crypto.DeriveKey(masterPassword, meta.Salt)
	if err != nil {
		return nil, err
	}
	if !key.MatchesVerifier(meta.Verifier) {
		key.Zero()
		return nil, crypto.ErrInvalidKey
	}
	return key, nil
}

// Register creates a remote account and stores tokens locally (encrypted with key).
func (s *Service) Register(ctx context.Context, login, password string, key *crypto.MasterKey) error {
	resp, err := s.conn.Auth.Register(ctx, gophkeeperv1.RegisterRequest_builder{
		Login:    login,
		Password: password,
	}.Build())
	if err != nil {
		return fmt.Errorf("auth: register: %w", err)
	}
	return s.Login(ctx, login, password, resp.GetUser().GetId(), key)
}

// Login authenticates remotely and persists an encrypted session.
func (s *Service) Login(ctx context.Context, login, password, knownUserID string, key *crypto.MasterKey) error {
	if key == nil {
		return fmt.Errorf("auth: master key required to seal tokens")
	}
	resp, err := s.conn.Auth.Login(ctx, gophkeeperv1.LoginRequest_builder{
		Login:    login,
		Password: password,
	}.Build())
	if err != nil {
		return fmt.Errorf("auth: login: %w", err)
	}
	userID := resp.GetUser().GetId()
	if userID == "" {
		userID = knownUserID
	}
	accessEnc, err := key.SealString(resp.GetAccessToken())
	if err != nil {
		return err
	}
	refreshEnc, err := key.SealString(resp.GetRefreshToken())
	if err != nil {
		return err
	}
	version, _ := s.vault.GetServerVersion(ctx)
	return s.vault.SaveSession(ctx, storage.Session{
		UserID:        userID,
		Login:         login,
		AccessToken:   accessEnc,
		RefreshToken:  refreshEnc,
		ServerVersion: version,
	})
}

// EnsureAccess returns a valid access token, refreshing when needed.
// Tokens are decrypted with the master key; plaintext is not persisted.
func (s *Service) EnsureAccess(ctx context.Context, key *crypto.MasterKey) (string, error) {
	if key == nil {
		return "", fmt.Errorf("auth: master key required")
	}
	session, err := s.vault.GetSession(ctx)
	if err != nil {
		return "", err
	}
	if len(session.AccessToken) == 0 && len(session.RefreshToken) == 0 {
		return "", fmt.Errorf("auth: not logged in")
	}
	if len(session.AccessToken) > 0 {
		access, err := key.OpenString(session.AccessToken)
		if err != nil {
			return "", fmt.Errorf("auth: decrypt access token: %w", err)
		}
		if accessStillValid(access, time.Now()) {
			return access, nil
		}
	}
	if len(session.RefreshToken) == 0 {
		return "", fmt.Errorf("auth: not logged in")
	}
	return s.refresh(ctx, session, key)
}

// accessStillValid reports whether the JWT access token is usable without refresh.
// Signature is not verified (client has no signing keys); only claim exp is read.
func accessStillValid(token string, now time.Time) bool {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return false
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return false
	}
	return now.Add(refreshSkew).Before(exp.Time)
}

func (s *Service) refresh(ctx context.Context, session storage.Session, key *crypto.MasterKey) (string, error) {
	refreshPlain, err := key.OpenBytes(session.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("auth: decrypt refresh token: %w", err)
	}
	defer secure.Zero(refreshPlain)

	resp, err := s.conn.Auth.Refresh(ctx, gophkeeperv1.RefreshRequest_builder{
		RefreshToken: string(refreshPlain),
	}.Build())
	if err != nil {
		return "", fmt.Errorf("auth: refresh: %w", err)
	}
	accessEnc, err := key.SealString(resp.GetAccessToken())
	if err != nil {
		return "", err
	}
	refreshEnc, err := key.SealString(resp.GetRefreshToken())
	if err != nil {
		return "", err
	}
	session.AccessToken = accessEnc
	session.RefreshToken = refreshEnc
	if err := s.vault.SaveSession(ctx, session); err != nil {
		return "", err
	}
	return resp.GetAccessToken(), nil
}
