package atrest

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestAESGCMRoundTrip(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0x42}, KeySize)
	c, err := NewAESGCM(key)
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("client-e2e-ciphertext")
	sealed, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sealed, plain) {
		t.Fatal("ciphertext must differ from plaintext")
	}

	got, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q, want %q", got, plain)
	}
}

func TestAESGCMFromHex(t *testing.T) {
	t.Parallel()

	hexKey := hex.EncodeToString(bytes.Repeat([]byte{0x11}, KeySize))
	c, err := NewAESGCMFromHex(hexKey)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Encrypt([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("got %q", got)
	}
}

func TestAESGCMInvalidKey(t *testing.T) {
	t.Parallel()

	if _, err := NewAESGCM([]byte("short")); err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestAESGCMInvalidCiphertext(t *testing.T) {
	t.Parallel()

	c, err := NewAESGCM(bytes.Repeat([]byte{1}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Decrypt([]byte("nope")); err != ErrInvalidCiphertext {
		t.Fatalf("want ErrInvalidCiphertext, got %v", err)
	}
}

func TestAESGCMTamper(t *testing.T) {
	t.Parallel()

	c, err := NewAESGCM(bytes.Repeat([]byte{7}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := c.Decrypt(sealed); err != ErrInvalidCiphertext {
		t.Fatalf("want ErrInvalidCiphertext, got %v", err)
	}
}
