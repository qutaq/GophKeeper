package model

import "time"

// Metadata is arbitrary textual metadata attached to an item
// (website, person, bank, activation codes, etc.).
type Metadata map[string]string

// Clone returns a shallow copy of m.
func (m Metadata) Clone() Metadata {
	if m == nil {
		return nil
	}
	out := make(Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Item is a private data record owned by a user.
// EncryptedData holds the client-side E2E ciphertext; the server never sees plaintext.
type Item struct {
	ID            string
	OwnerID       string
	Type          DataType
	EncryptedData []byte
	Metadata      Metadata
	Version       int64
	UpdatedAt     time.Time
	Deleted       bool
}
