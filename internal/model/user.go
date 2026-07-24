// Package model contains domain types shared by server and client.
package model

import "time"

// User is a registered account.
type User struct {
	ID           string
	Login        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
