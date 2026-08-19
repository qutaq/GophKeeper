// Package atrest provides server-side encryption of stored payloads.
// Client E2E ciphertext is encrypted again before it hits the database.
package atrest

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32
	nonceSize = 12
)

var (
	// ErrInvalidKey is returned when the provided key length is not KeySize.
	ErrInvalidKey = errors.New("atrest: key must be 32 bytes")
	// ErrInvalidCiphertext is returned when ciphertext is too short or corrupt.
	ErrInvalidCiphertext = errors.New("atrest: invalid ciphertext")
)

// Cipher encrypts and decrypts opaque blobs at rest.
// Implementations may wrap a local DEK or a KMS envelope.
type Cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AESGCM is a local AES-256-GCM Cipher (nonce || ciphertext||tag).
type AESGCM struct {
	aead cipher.AEAD
}

// NewAESGCM builds a Cipher from a raw 32-byte key.
func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("atrest: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("atrest: gcm: %w", err)
	}
	return &AESGCM{aead: aead}, nil
}

// NewAESGCMFromHex builds a Cipher from a hex-encoded 32-byte key.
func NewAESGCMFromHex(hexKey string) (*AESGCM, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("atrest: decode key: %w", err)
	}
	return NewAESGCM(key)
}

// Encrypt seals plaintext. Output layout: nonce || sealed.
func (c *AESGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("atrest: nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, nonceSize+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt opens a blob previously produced by Encrypt.
func (c *AESGCM) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize+c.aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	nonce := ciphertext[:nonceSize]
	sealed := ciphertext[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plain, nil
}

// Ensure AESGCM implements Cipher.
var _ Cipher = (*AESGCM)(nil)
