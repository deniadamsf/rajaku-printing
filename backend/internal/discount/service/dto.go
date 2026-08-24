package service

import (
	"time"

	"github.com/google/uuid"
)

// CreateInput — payload untuk membuat master diskon baru (§28.1).
type CreateInput struct {
	Code              string
	Name              string
	Type              string // "percent" | "nominal"
	ValuePercent      *float64
	ValueAmount       *int64
	MaxDiscountAmount *int64
	MinSubtotal       int64
	StartsAt          *time.Time
	EndsAt            *time.Time
	Quota             *int
	ChannelScope      string // "all" | "online" | "pos"; "" = default "all"
	IsActive          bool
}

// UpdateInput — payload untuk PATCH master diskon. Field pointer biasa = nil
// artinya "jangan sentuh". Untuk kolom yang boleh diisi NULL secara sengaja
// (MaxDiscountAmount, StartsAt, EndsAt, Quota) disediakan flag Clear* — HANYA
// dipakai kalau handler mendeteksi request body eksplisit mengirim
// `"field": null` (dibedakan dari field yang sama sekali tidak dikirim).
type UpdateInput struct {
	Code *string
	Name *string

	Type         *string
	ValuePercent *float64
	ValueAmount  *int64

	MaxDiscountAmount      *int64
	ClearMaxDiscountAmount bool

	MinSubtotal *int64

	StartsAt      *time.Time
	ClearStartsAt bool
	EndsAt        *time.Time
	ClearEndsAt   bool

	Quota      *int
	ClearQuota bool

	ChannelScope *string
	IsActive     *bool
}

// ListFilter — filter untuk GET /admin/discounts.
type ListFilter struct {
	Status  string // status turunan: aktif|terjadwal|kadaluarsa|nonaktif|kuota_habis; "" = semua
	Query   string // cari di code/name
	Page    int
	PerPage int
}

// DiscountView — proyeksi diskon utk admin panel, termasuk status turunan &
// usage_count (dihitung, bukan disimpan — §28.4). JSON tag mengikuti bentuk
// yang dipakai frontend (§28 brief) — dikembalikan langsung oleh handler,
// tanpa DTO tambahan (pola sama seperti pos.service.CreateOrderResult).
type DiscountView struct {
	ID                uuid.UUID  `json:"id"`
	Code              string     `json:"code"`
	Name              string     `json:"name"`
	Type              string     `json:"type"`
	ValuePercent      *float64   `json:"value_percent"`
	ValueAmount       *int64     `json:"value_amount"`
	MaxDiscountAmount *int64     `json:"max_discount_amount"`
	MinSubtotal       int64      `json:"min_subtotal"`
	StartsAt          *time.Time `json:"starts_at"`
	EndsAt            *time.Time `json:"ends_at"`
	Quota             *int       `json:"quota"`
	UsageCount        int64      `json:"usage_count"`
	ChannelScope      string     `json:"channel_scope"`
	IsActive          bool       `json:"is_active"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ApplicableView — DiscountView + preview_amount, dipakai layar kasir
// (GET /admin/discounts/applicable, §28.5 UI).
type ApplicableView struct {
	DiscountView
	PreviewAmount int64 `json:"preview_amount"`
}

// ListResult — halaman hasil List.
type ListResult struct {
	Items   []DiscountView `json:"items"`
	Total   int64          `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}
