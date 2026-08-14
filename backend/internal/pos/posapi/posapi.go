// Package posapi is the PUBLIC contract of the POS module (§22).
// Modul ini sepenuhnya user-facing (admin panel) — belum ada modul lain yang
// consume. Package ini menampung sentinel errors.
package posapi

import "errors"

var (
	ErrInvalidPhone           = errors.New("posapi: nomor WA pelanggan tidak valid")
	ErrCustomerResolve        = errors.New("posapi: gagal resolve/create customer dari nomor WA")
	ErrInvalidMetodeBayar     = errors.New("posapi: metode_bayar POS harus cash atau qris_pos")
	ErrOrderCreate            = errors.New("posapi: gagal create walk-in order")
	ErrInvalidDate            = errors.New("posapi: tanggal rekonsiliasi invalid")
)
