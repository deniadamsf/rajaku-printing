// Package service — production flow orchestration.
package service

import "github.com/google/uuid"

// AdvanceInput — payload untuk semua transisi produksi non-shipping.
type AdvanceInput struct {
	Resi    string
	StaffID uuid.UUID
	Note    string
}

// MarkShippedInput — khusus dikirim (butuh kurir + tracking).
type MarkShippedInput struct {
	Resi           string
	StaffID        uuid.UUID
	Courier        string
	TrackingNumber string
	Note           string
}
