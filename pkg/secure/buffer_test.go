package secure

import (
	"bytes"
	"testing"
)

func TestConsumeZerosSource(t *testing.T) {
	t.Parallel()

	src := []byte("secret-password")
	buf := Consume(src)
	defer buf.Zero()

	if !bytes.Equal(src, make([]byte, len(src))) {
		t.Fatalf("source not zeroed: %q", src)
	}
	if got := string(buf.Bytes()); got != "secret-password" {
		t.Fatalf("got %q", got)
	}
}

func TestZero(t *testing.T) {
	t.Parallel()

	buf := Clone([]byte("abc"))
	buf.Zero()
	if !bytes.Equal(buf.Bytes(), []byte{0, 0, 0}) {
		t.Fatalf("not zeroed: %v", buf.Bytes())
	}
}

func TestLockBestEffort(t *testing.T) {
	t.Parallel()

	buf := Clone([]byte("locked-secret-material!!"))
	defer buf.Zero()
	// May fail without privileges; must not panic.
	_ = buf.Lock()
}
