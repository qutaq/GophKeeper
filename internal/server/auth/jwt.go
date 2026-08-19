package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType distinguishes access vs refresh JWT claims.
type TokenType string

const (
	// TokenTypeAccess is a short-lived API access token.
	TokenTypeAccess TokenType = "access"
	// TokenTypeRefresh is a long-lived refresh token.
	TokenTypeRefresh TokenType = "refresh"
)

// Claims are JWT claims used by GophKeeper.
type Claims struct {
	UserID string    `json:"uid"`
	Type   TokenType `json:"typ"`
	jwt.RegisteredClaims
}

// SigningKey is one HMAC signing key identified by kid.
type SigningKey struct {
	ID  string
	Key []byte
}

// KeyRing holds the current signing key and previous keys for verification (rotation).
type KeyRing struct {
	mu      sync.RWMutex
	current SigningKey
	keys    map[string]SigningKey
}

// NewKeyRing builds a key ring. current must be present in keys.
func NewKeyRing(current SigningKey, previous ...SigningKey) (*KeyRing, error) {
	if current.ID == "" || len(current.Key) == 0 {
		return nil, fmt.Errorf("auth: empty current signing key")
	}
	keys := map[string]SigningKey{current.ID: current}
	for _, k := range previous {
		if k.ID == "" || len(k.Key) == 0 {
			return nil, fmt.Errorf("auth: empty previous signing key")
		}
		keys[k.ID] = k
	}
	return &KeyRing{current: current, keys: keys}, nil
}

// ParseKeyRingEnv parses JWT_KEYS=kid1:hex,kid2:hex and JWT_CURRENT_KID=kid2.
func ParseKeyRingEnv(keysCSV, currentKID string) (*KeyRing, error) {
	if keysCSV == "" {
		return nil, fmt.Errorf("auth: JWT_KEYS is required")
	}
	var parsed []SigningKey
	for _, part := range strings.Split(keysCSV, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kid, hexKey, ok := strings.Cut(part, ":")
		if !ok || kid == "" || hexKey == "" {
			return nil, fmt.Errorf("auth: invalid JWT_KEYS entry %q", part)
		}
		key, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("auth: decode JWT key %s: %w", kid, err)
		}
		if len(key) < 32 {
			return nil, fmt.Errorf("auth: JWT key %s must be at least 32 bytes", kid)
		}
		parsed = append(parsed, SigningKey{ID: kid, Key: key})
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("auth: no JWT keys configured")
	}
	if currentKID == "" {
		currentKID = parsed[len(parsed)-1].ID
	}
	var current SigningKey
	var previous []SigningKey
	for _, k := range parsed {
		if k.ID == currentKID {
			current = k
			continue
		}
		previous = append(previous, k)
	}
	if current.ID == "" {
		return nil, fmt.Errorf("auth: JWT_CURRENT_KID %q not found in JWT_KEYS", currentKID)
	}
	return NewKeyRing(current, previous...)
}

// Rotate installs a new current key while keeping old keys for verification.
func (r *KeyRing) Rotate(next SigningKey) error {
	if next.ID == "" || len(next.Key) == 0 {
		return fmt.Errorf("auth: empty next signing key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[next.ID] = next
	r.current = next
	return nil
}

// CurrentKID returns the active signing key id.
func (r *KeyRing) CurrentKID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current.ID
}

// TokenManager issues and validates JWTs.
type TokenManager struct {
	keys       *KeyRing
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// TokenManagerConfig configures TokenManager.
type TokenManagerConfig struct {
	Keys       *KeyRing
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// NewTokenManager constructs a TokenManager.
func NewTokenManager(cfg TokenManagerConfig) (*TokenManager, error) {
	if cfg.Keys == nil {
		return nil, fmt.Errorf("auth: nil key ring")
	}
	if cfg.AccessTTL <= 0 {
		return nil, fmt.Errorf("auth: AccessTTL must be positive")
	}
	if cfg.RefreshTTL <= 0 {
		return nil, fmt.Errorf("auth: RefreshTTL must be positive")
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "gophkeeper"
	}
	return &TokenManager{
		keys:       cfg.Keys,
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
	}, nil
}

// IssueAccess creates a signed access JWT for userID.
func (m *TokenManager) IssueAccess(userID string) (token string, expiresAt time.Time, err error) {
	return m.issue(userID, TokenTypeAccess, m.accessTTL)
}

// IssueRefreshJWT creates a signed refresh JWT for userID.
func (m *TokenManager) IssueRefreshJWT(userID string) (token string, expiresAt time.Time, err error) {
	return m.issue(userID, TokenTypeRefresh, m.refreshTTL)
}

func (m *TokenManager) issue(userID string, typ TokenType, ttl time.Duration) (string, time.Time, error) {
	now := time.Now().UTC()
	expires := now.Add(ttl)
	claims := Claims{
		UserID: userID,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}

	m.keys.mu.RLock()
	current := m.keys.current
	m.keys.mu.RUnlock()

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t.Header["kid"] = current.ID
	signed, err := t.SignedString(current.Key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expires, nil
}

// ParseAndValidate parses a JWT and ensures it matches wantType.
func (m *TokenManager) ParseAndValidate(tokenString string, wantType TokenType) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("auth: unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		m.keys.mu.RLock()
		defer m.keys.mu.RUnlock()
		if kid != "" {
			if k, ok := m.keys.keys[kid]; ok {
				return k.Key, nil
			}
			return nil, fmt.Errorf("auth: unknown kid")
		}
		// Fallback: try current key when kid is missing (legacy tokens).
		return m.keys.current.Key, nil
	}, jwt.WithIssuer(m.issuer))
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Type != wantType || claims.UserID == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// AccessTTL returns configured access token lifetime.
func (m *TokenManager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL returns configured refresh token lifetime.
func (m *TokenManager) RefreshTTL() time.Duration { return m.refreshTTL }

// NewOpaqueRefreshToken returns a high-entropy opaque refresh token string.
func NewOpaqueRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
