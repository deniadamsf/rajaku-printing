package model

import (
	"time"

	"github.com/google/uuid"
)

// OrderItem — satu baris produk di dalam sebuah order (§32). Sampai
// migration 000033, satu order = satu produk (kolom item menempel langsung
// di `orders`); sejak migration itu order boleh memuat banyak produk/ukuran
// sekaligus dengan SATU resi, SATU ongkir, SATU diskon — dibedakan `LineNo`
// (mulai dari 1, unik per order).
//
// Field uang di sini (UnitPrice, Subtotal, DiscountAmount) HANYA untuk
// menjelaskan angka agregat di `orders` per baris — layar uang (rekap,
// invoice, struk) tetap membaca orders.Subtotal/orders.DiscountAmount,
// TIDAK PERNAH menjumlahkan tabel ini (§32.2, sambungan aturan snapshot
// §28.2 lapis 3). Dua invarian yang wajib dijaga service:
//
//	Σ item.Subtotal        == orders.Subtotal
//	Σ item.DiscountAmount  == orders.DiscountAmount
type OrderItem struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	// OrderID — FK ke orders, TANPA ON DELETE CASCADE: order tidak pernah
	// di-hard-delete (soft delete saja, migration 000025), jadi item-nya
	// tidak pernah perlu dihapus otomatis lewat cascade.
	OrderID uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	// LineNo — urutan tampil, mulai dari 1. UNIQUE (order_id, line_no) di DB.
	LineNo int `gorm:"not null" json:"line_no"`

	// Product snapshot (sama pola dgn orders sebelumnya — denormalized
	// supaya immun terhadap perubahan katalog belakangan).
	ProductID            *uuid.UUID `gorm:"type:uuid"                                       json:"product_id,omitempty"`
	ProductNameSnapshot  string     `gorm:"size:255;not null;column:product_name_snapshot"  json:"product_name_snapshot"`
	MaterialID           *uuid.UUID `gorm:"type:uuid"                                       json:"material_id,omitempty"`
	MaterialNameSnapshot string     `gorm:"size:255;not null;column:material_name_snapshot" json:"material_name_snapshot"`
	PricingTypeSnapshot  string     `gorm:"size:20;not null;column:pricing_type_snapshot"   json:"pricing_type_snapshot"`
	WidthCm              int        `gorm:"not null"                                        json:"width_cm"`
	HeightCm             int        `gorm:"not null"                                        json:"height_cm"`
	Quantity             int        `gorm:"not null;default:1"                              json:"quantity"`
	UnitPrice            int64      `gorm:"not null"                                        json:"unit_price"`
	Subtotal             int64      `gorm:"not null"                                        json:"subtotal"`

	// DiscountAmount — bagian dari orders.DiscountAmount yang jatuh ke baris
	// ini (§32.3, metode sisa terbesar). Murni penjelas, bukan sumber
	// kebenaran uang (lihat doc struct).
	DiscountAmount int64 `gorm:"not null;default:0;column:discount_amount" json:"discount_amount"`

	// Desain PER ITEM (§32.5) — satu order boleh campur: item A bawa desain
	// sendiri, item B minta dibuatkan. orders.DesignSource (upload|request|
	// mixed) adalah TURUNAN dari kumpulan field ini — jangan pernah
	// memvalidasi file desain terhadap orders.DesignSource, selalu terhadap
	// milik baris item-nya sendiri.
	DesignSource DesignSource `gorm:"size:20;not null;column:design_source" json:"design_source"`
	DesignBrief  *string      `gorm:"column:design_brief"                   json:"design_brief,omitempty"`
	ItemNotes    *string      `gorm:"column:item_notes"                     json:"item_notes,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (OrderItem) TableName() string { return "order_items" }
