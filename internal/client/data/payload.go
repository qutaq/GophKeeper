// Package data implements client-side vault item CRUD with E2E encryption
// and typed payload helpers for the CLI.
package data

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/qutaq/gophkeeper/internal/model"
)

// CredentialsPayload is a login/password secret.
type CredentialsPayload struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// TextPayload is arbitrary text.
type TextPayload struct {
	Text string `json:"text"`
}

// BinaryPayload is arbitrary binary content.
type BinaryPayload struct {
	Filename string `json:"filename"`
	Data     string `json:"data"` // base64
}

// CardPayload is bank card data.
type CardPayload struct {
	Number string `json:"number"`
	Holder string `json:"holder"`
	Exp    string `json:"exp"`
	CVV    string `json:"cvv"`
}

// OTPPayload stores a TOTP secret.
type OTPPayload struct {
	Secret  string `json:"secret"`
	Issuer  string `json:"issuer"`
	Account string `json:"account"`
}

// MarshalPayload encodes a typed payload to JSON bytes.
func MarshalPayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

// ParseType converts a CLI type name to model.DataType.
func ParseType(name string) (model.DataType, error) {
	switch name {
	case "credentials", "cred", "password":
		return model.DataTypeCredentials, nil
	case "text":
		return model.DataTypeText, nil
	case "binary", "bin", "file":
		return model.DataTypeBinary, nil
	case "card":
		return model.DataTypeCard, nil
	case "otp":
		return model.DataTypeOTP, nil
	default:
		return model.DataTypeUnspecified, fmt.Errorf("unknown type %q", name)
	}
}

// DecodeBinary builds a BinaryPayload from raw bytes.
func DecodeBinary(filename string, raw []byte) BinaryPayload {
	return BinaryPayload{
		Filename: filename,
		Data:     base64.StdEncoding.EncodeToString(raw),
	}
}

// EncodeBinary extracts raw bytes from a BinaryPayload.
func EncodeBinary(p BinaryPayload) ([]byte, error) {
	return base64.StdEncoding.DecodeString(p.Data)
}

// FormatPayload returns a human-readable plaintext view.
func FormatPayload(typ model.DataType, plain []byte) (string, error) {
	switch typ {
	case model.DataTypeCredentials:
		var p CredentialsPayload
		if err := json.Unmarshal(plain, &p); err != nil {
			return "", err
		}
		return fmt.Sprintf("login: %s\npassword: %s", p.Login, p.Password), nil
	case model.DataTypeText:
		var p TextPayload
		if err := json.Unmarshal(plain, &p); err != nil {
			return "", err
		}
		return p.Text, nil
	case model.DataTypeBinary:
		var p BinaryPayload
		if err := json.Unmarshal(plain, &p); err != nil {
			return "", err
		}
		raw, err := EncodeBinary(p)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("filename: %s\nsize: %d bytes", p.Filename, len(raw)), nil
	case model.DataTypeCard:
		var p CardPayload
		if err := json.Unmarshal(plain, &p); err != nil {
			return "", err
		}
		return fmt.Sprintf("number: %s\nholder: %s\nexp: %s\ncvv: %s", p.Number, p.Holder, p.Exp, p.CVV), nil
	case model.DataTypeOTP:
		var p OTPPayload
		if err := json.Unmarshal(plain, &p); err != nil {
			return "", err
		}
		return fmt.Sprintf("secret: %s\nissuer: %s\naccount: %s", p.Secret, p.Issuer, p.Account), nil
	default:
		return string(plain), nil
	}
}
