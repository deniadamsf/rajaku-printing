// Package service holds the auth business logic. Handler-facing DTOs live here
// (input/output structs) so the handler stays thin.
package service

import (
	"time"

	"github.com/google/uuid"
)

type RegisterCustomerInput struct {
	Email    string
	Phone    string // raw — service will normalize
	Name     string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

type TokenPair struct {
	AccessToken string
	TokenType   string        // "Bearer"
	ExpiresIn   time.Duration // access token TTL
}

type MeOutput struct {
	UserID      uuid.UUID `json:"user_id"`
	UserType    string    `json:"user_type"`
	Email       string    `json:"email,omitempty"`
	Phone       string    `json:"phone"`
	Name        string    `json:"name"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"permissions"`
}
