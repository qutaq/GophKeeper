package data

import (
	"strings"
	"testing"

	"github.com/qutaq/gophkeeper/internal/model"
)

func TestParseTypeAndFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		typ  string
		raw  any
		want string
	}{
		{"credentials", "credentials", CredentialsPayload{Login: "u", Password: "p"}, "login: u"},
		{"text", "text", TextPayload{Text: "hello"}, "hello"},
		{"card", "card", CardPayload{Number: "1", Holder: "A", Exp: "01/30", CVV: "9"}, "number: 1"},
		{"otp", "otp", OTPPayload{Secret: "s", Issuer: "i", Account: "a"}, "secret: s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			typ, err := ParseType(tc.typ)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := MarshalPayload(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			view, err := FormatPayload(typ, plain)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(view, tc.want) {
				t.Fatalf("view=%q want substring %q", view, tc.want)
			}
		})
	}
}

func TestBinaryRoundTrip(t *testing.T) {
	t.Parallel()

	p := DecodeBinary("a.bin", []byte{1, 2, 3})
	raw, err := EncodeBinary(p)
	if err != nil || len(raw) != 3 {
		t.Fatalf("raw=%v err=%v", raw, err)
	}
	plain, err := MarshalPayload(p)
	if err != nil {
		t.Fatal(err)
	}
	view, err := FormatPayload(model.DataTypeBinary, plain)
	if err != nil || !strings.Contains(view, "a.bin") {
		t.Fatalf("view=%q err=%v", view, err)
	}
}

func TestParseTypeRejectsUnknown(t *testing.T) {
	t.Parallel()
	if _, err := ParseType("nope"); err == nil {
		t.Fatal("expected error")
	}
}
