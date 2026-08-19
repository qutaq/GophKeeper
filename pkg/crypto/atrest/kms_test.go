package atrest

import (
	"bytes"
	"context"
	"testing"
)

func TestNewCipherFromProvider(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{0xAB}, KeySize)
	c, err := NewCipherFromProvider(context.Background(), StaticKey{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q", got)
	}
}

func TestStaticKeyInvalid(t *testing.T) {
	t.Parallel()

	_, err := NewCipherFromProvider(context.Background(), StaticKey{Key: []byte("x")})
	if err == nil {
		t.Fatal("expected error")
	}
}
