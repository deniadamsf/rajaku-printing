// Package productionapi is the PUBLIC contract of the production module (§22).
// Modul ini thin — mendelegasikan state transitions ke orderapi + trigger
// notifikasi WA. Belum ada consumer eksternal; api ini terutama menampung
// sentinel errors + dokumentasi kontrak.
package productionapi

import "errors"

var (
	ErrOrderNotFound            = errors.New("productionapi: order not found")
	ErrInvalidTransition        = errors.New("productionapi: transisi tidak valid dari status saat ini")
	ErrOrderStateChanged        = errors.New("productionapi: order state changed concurrently — refresh & retry")
	ErrShippingTrackingRequired = errors.New("productionapi: kurir & no. resi ekspedisi wajib diisi saat mark dikirim")
	ErrDikirimOnPickup          = errors.New("productionapi: mark-shipped hanya untuk metode_ambil=kirim")
)
