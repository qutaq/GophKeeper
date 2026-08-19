// Package crypto provides client-side KDF and E2E encryption for vault payloads.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"

	"github.com/qutaq/gophkeeper/pkg/secure"
)

const (
	// KeySize is AES-256 key length.
	KeySize   = 32
	saltSize  = 16
	nonceSize = 12

	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
)

// ErrInvalidKey is returned for wrong master password / corrupt ciphertext.
var ErrInvalidKey = errors.New("crypto: invalid key or ciphertext")

// MasterKey is a derived AES-256 key from a master password.
// Key material lives in a secure.Buffer (best-effort mlock + explicit Zero).
type MasterKey struct {
	buf *secure.Buffer
}

// DeriveKey derives a MasterKey from password and salt using argon2id.
// password is consumed (zeroed) after derivation.
func DeriveKey(password []byte, salt []byte) (*MasterKey, error) {
	if len(salt) != saltSize {
		return nil, fmt.Errorf("crypto: salt must be %d bytes", saltSize)
	}
	defer secure.Zero(password)

	raw := argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, KeySize)
	buf := secure.Consume(raw)
	_ = buf.Lock() // best-effort; ignore privilege errors
	return &MasterKey{buf: buf}, nil
}

// SaltFromLogin returns a deterministic 16-byte salt for cross-device key agreement.
func SaltFromLogin(login string) []byte {
	sum := sha256.Sum256([]byte("gophkeeper/salt/v1:" + login))
	return sum[:saltSize]
}

// NewSalt returns a random KDF salt.
func NewSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("crypto: salt: %w", err)
	}
	return salt, nil
}

func (k *MasterKey) key() []byte {
	if k == nil || k.buf == nil {
		return nil
	}
	return k.buf.Bytes()
}

// Verifier returns a non-secret check value to validate the master key later.
func (k *MasterKey) Verifier() string {
	sum := sha256.Sum256(append([]byte("gophkeeper-verifier:"), k.key()...))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

// MatchesVerifier reports whether verifier was produced by this key.
func (k *MasterKey) MatchesVerifier(verifier string) bool {
	want, err := base64.RawStdEncoding.DecodeString(verifier)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	sum := sha256.Sum256(append([]byte("gophkeeper-verifier:"), k.key()...))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

// Encrypt seals plaintext with AES-GCM. Output: nonce || ciphertext||tag.
func (k *MasterKey) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key())
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, nonceSize+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt opens a blob produced by Encrypt.
func (k *MasterKey) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize+16 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(k.key())
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return plain, nil
}

// SealString encrypts a UTF-8 string for local persistence (e.g. auth tokens).
func (k *MasterKey) SealString(plain string) ([]byte, error) {
	return k.Encrypt([]byte(plain))
}

// OpenBytes decrypts a blob produced by Encrypt / SealString.
func (k *MasterKey) OpenBytes(ciphertext []byte) ([]byte, error) {
	return k.Decrypt(ciphertext)
}

// OpenString decrypts a blob produced by SealString.
func (k *MasterKey) OpenString(ciphertext []byte) (string, error) {
	plain, err := k.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	s := string(plain)
	secure.Zero(plain)
	return s, nil
}

// Zero overwrites the key material in memory.
func (k *MasterKey) Zero() {
	if k == nil || k.buf == nil {
		return
	}
	k.buf.Zero()
	k.buf = nil
}
