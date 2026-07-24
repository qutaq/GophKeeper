package model

import "testing"

func TestDataTypeString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   DataType
		want string
	}{
		{DataTypeUnspecified, "DATA_TYPE_UNSPECIFIED"},
		{DataTypeCredentials, "CREDENTIALS"},
		{DataTypeText, "TEXT"},
		{DataTypeBinary, "BINARY"},
		{DataTypeCard, "CARD"},
		{DataTypeOTP, "OTP"},
		{DataType(99), "DATA_TYPE_UNSPECIFIED"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Fatalf("%v.String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDataTypeValid(t *testing.T) {
	t.Parallel()

	if DataTypeUnspecified.Valid() {
		t.Fatal("unspecified must be invalid")
	}
	for _, dt := range []DataType{DataTypeCredentials, DataTypeText, DataTypeBinary, DataTypeCard, DataTypeOTP} {
		if !dt.Valid() {
			t.Fatalf("%v must be valid", dt)
		}
	}
	if DataType(99).Valid() {
		t.Fatal("unknown type must be invalid")
	}
}

func TestMetadataClone(t *testing.T) {
	t.Parallel()

	if Metadata(nil).Clone() != nil {
		t.Fatal("nil clone must be nil")
	}

	src := Metadata{"site": "example.com", "owner": "alice"}
	dst := src.Clone()
	dst["site"] = "changed"
	if src["site"] != "example.com" {
		t.Fatal("clone must be independent")
	}
}
