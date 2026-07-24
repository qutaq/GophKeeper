package crypto

import (
	"bytes"
	"testing"
)

func TestMasterKeyRoundTrip(t *testing.T) {
	t.Parallel()

	salt := SaltFromLogin("alice")
	key, err := DeriveKey([]byte("master-pass"), salt)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()

	verifier := key.Verifier()
	if !key.MatchesVerifier(verifier) {
		t.Fatal("verifier mismatch")
	}

	wrong, err := DeriveKey([]byte("other"), salt)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Zero()
	if wrong.MatchesVerifier(verifier) {
		t.Fatal("wrong password must not match verifier")
	}

	sealed, err := key.Encrypt([]byte(`{"login":"a","password":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := key.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, []byte(`{"login":"a","password":"b"}`)) {
		t.Fatalf("got %s", plain)
	}
	if _, err := wrong.Decrypt(sealed); err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}

	tok, err := key.SealString("access-token-value")
	if err != nil {
		t.Fatal(err)
	}
	got, err := key.OpenString(tok)
	if err != nil || got != "access-token-value" {
		t.Fatalf("seal/open: got %q err=%v", got, err)
	}
	if _, err := wrong.OpenString(tok); err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey for wrong key, got %v", err)
	}

	otherSalt := SaltFromLogin("bob")
	if bytes.Equal(salt, otherSalt) {
		t.Fatal("salts for different logins must differ")
	}
}

func TestDeriveKeyConsumesPassword(t *testing.T) {
	t.Parallel()

	pass := []byte("wipe-me-please")
	salt := SaltFromLogin("user")
	key, err := DeriveKey(pass, salt)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Zero()
	if !bytes.Equal(pass, make([]byte, len(pass))) {
		t.Fatalf("password not zeroed: %q", pass)
	}
}
