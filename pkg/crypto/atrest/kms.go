package atrest

import (
	"context"
	"fmt"
)

// KeyProvider supplies a data-encryption key (DEK).
// Local env keys or a remote KMS can implement this interface.
type KeyProvider interface {
	// DEK returns a 32-byte AES key used to seal payloads at rest.
	DEK(ctx context.Context) ([]byte, error)
}

// StaticKey is a KeyProvider that always returns the same DEK.
type StaticKey struct {
	Key []byte
}

// DEK returns the configured key.
func (s StaticKey) DEK(ctx context.Context) ([]byte, error) {
	_ = ctx
	if len(s.Key) != KeySize {
		return nil, ErrInvalidKey
	}
	out := make([]byte, KeySize)
	copy(out, s.Key)
	return out, nil
}

// NewCipherFromProvider builds an AES-GCM Cipher from a KeyProvider.
// This is the seam where a KMS-backed provider can be swapped in later.
func NewCipherFromProvider(ctx context.Context, provider KeyProvider) (Cipher, error) {
	if provider == nil {
		return nil, fmt.Errorf("atrest: nil key provider")
	}
	key, err := provider.DEK(ctx)
	if err != nil {
		return nil, fmt.Errorf("atrest: dek: %w", err)
	}
	return NewAESGCM(key)
}
