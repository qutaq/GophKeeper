// Package auth implements registration, login, JWT issuance and refresh-token handling.
package auth

import "errors"

var (
	// ErrInvalidCredentials is returned when login/password do not match.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("auth: invalid input")
	// ErrLoginTaken is returned when a login is already registered.
	ErrLoginTaken = errors.New("auth: login already taken")
	// ErrInvalidToken is returned for malformed/expired/revoked tokens.
	ErrInvalidToken = errors.New("auth: invalid token")
)
