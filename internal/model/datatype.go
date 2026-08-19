package model

// DataType enumerates supported private data kinds.
type DataType int

const (
	// DataTypeUnspecified is an unset / invalid type.
	DataTypeUnspecified DataType = iota
	// DataTypeCredentials is a login/password pair.
	DataTypeCredentials
	// DataTypeText is arbitrary text.
	DataTypeText
	// DataTypeBinary is arbitrary binary payload.
	DataTypeBinary
	// DataTypeCard is bank card data.
	DataTypeCard
	// DataTypeOTP is a one-time password (TOTP) secret.
	DataTypeOTP
)

// String returns a stable wire/storage name for the data type.
func (t DataType) String() string {
	switch t {
	case DataTypeCredentials:
		return "CREDENTIALS"
	case DataTypeText:
		return "TEXT"
	case DataTypeBinary:
		return "BINARY"
	case DataTypeCard:
		return "CARD"
	case DataTypeOTP:
		return "OTP"
	default:
		return "DATA_TYPE_UNSPECIFIED"
	}
}

// Valid reports whether t is a known non-unspecified type.
func (t DataType) Valid() bool {
	return t >= DataTypeCredentials && t <= DataTypeOTP
}
