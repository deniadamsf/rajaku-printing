package service

import (
	"time"

	"github.com/google/uuid"
)

// MembershipView — hasil transport dari service ke handler, satu proyeksi
// untuk semua endpoint (Apply/Approve/Reject/Revoke/List item) — sama pola
// dengan discount.DiscountView.
type MembershipView struct {
	CustomerID   uuid.UUID  `json:"customer_id"`
	Name         string     `json:"name"`
	Phone        string     `json:"phone"`
	Status       string     `json:"status"`
	RequestedAt  *time.Time `json:"requested_at,omitempty"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	DecidedBy    *uuid.UUID `json:"decided_by,omitempty"`
	DecisionNote string     `json:"decision_note,omitempty"`
}

// ListFilter/ListResult — GET /admin/membership (§30.2).
type ListFilter struct {
	Status  string
	Page    int
	PerPage int
}

type ListResult struct {
	Items   []MembershipView `json:"items"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}
