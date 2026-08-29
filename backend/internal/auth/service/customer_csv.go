// CSV row rendering untuk GET /admin/customers-export — file terpisah per
// §22 (satu tanggung jawab per file). Kolom & urutan lihat customerCSVHeader.
package service

import (
	"encoding/csv"
	"io"
	"time"

	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/pkg/csvsafe"
)

// customerCSVHeader — Nama, No WA, WA Terverifikasi, Email, Tipe, Status
// Akun, Status Member, Terakhir Login, Terdaftar (brief fitur ini).
var customerCSVHeader = []string{
	"Nama", "No WA", "WA Terverifikasi", "Email", "Tipe",
	"Status Akun", "Status Member", "Terakhir Login", "Terdaftar",
}

// customerCSVWriter wraps encoding/csv.Writer — mengalir langsung ke io.Writer
// yang dipasok caller (pola sama recap_handler.go: caller yang memutuskan
// kapan flush ke jaringan, service ini cuma menulis baris).
type customerCSVWriter struct {
	w *csv.Writer
}

func newCustomerCSVWriter(w io.Writer) *customerCSVWriter {
	return &customerCSVWriter{w: csv.NewWriter(w)}
}

func (c *customerCSVWriter) writeHeader() error {
	return c.w.Write(customerCSVHeader)
}

func (c *customerCSVWriter) writeRow(u *model.User) error {
	return c.w.Write(customerCSVRecord(u))
}

func (c *customerCSVWriter) flush() { c.w.Flush() }

func (c *customerCSVWriter) err() error { return c.w.Error() }

func customerCSVRecord(u *model.User) []string {
	phoneStr := ""
	if u.Phone != nil {
		phoneStr = *u.Phone
	}
	verified := "Tidak"
	if u.PhoneVerifiedAt != nil {
		verified = "Ya"
	}
	emailStr := ""
	if u.Email != nil {
		emailStr = *u.Email
	}
	customerType := ""
	if u.CustomerType != nil {
		customerType = string(*u.CustomerType)
	}
	accountStatus := "Nonaktif"
	if u.IsActive {
		accountStatus = "Aktif"
	}
	lastLogin := ""
	if u.LastLoginAt != nil {
		lastLogin = u.LastLoginAt.UTC().Format(time.RFC3339)
	}
	return []string{
		// csvsafe.Field — Nama sepenuhnya diisi pihak luar (pelanggan
		// mengetik namanya sendiri saat registrasi/guest checkout, §11), jadi
		// wajib dinetralkan dari formula injection sebelum ditulis sebagai
		// sel CSV (temuan review #6).
		csvsafe.Field(u.Name),
		phoneStr,
		verified,
		emailStr,
		customerType,
		accountStatus,
		string(u.MembershipStatus),
		lastLogin,
		u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
