package postgres

import (
	"strings"
	"testing"
)

func TestToMigrateURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"postgres://u:p@localhost:5432/db?sslmode=disable", "pgx5://u:p@localhost:5432/db?sslmode=disable"},
		{"postgresql://u:p@localhost/db", "pgx5://u:p@localhost/db"},
		{"pgx5://already", "pgx5://already"},
	}
	for _, tc := range cases {
		if got := ToMigrateURL(tc.in); got != tc.want {
			t.Fatalf("ToMigrateURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFileSourceURL(t *testing.T) {
	t.Parallel()

	url, err := fileSourceURL(".")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "file://") {
		t.Fatalf("unexpected source url: %s", url)
	}
}

func TestMarshalMetadata(t *testing.T) {
	t.Parallel()

	b, err := marshalMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("got %s", b)
	}

	m, err := unmarshalMetadata([]byte(`{"site":"ex.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["site"] != "ex.com" {
		t.Fatalf("got %#v", m)
	}
}
