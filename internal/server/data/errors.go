// Package data implements private item CRUD use-cases for the server.
package data

import "errors"

var (
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("data: invalid input")
	// ErrNotFound is returned when an item is missing or not owned by the caller.
	ErrNotFound = errors.New("data: not found")
	// ErrConflict is returned when optimistic concurrency fails (server version wins).
	ErrConflict = errors.New("data: version conflict")
)
