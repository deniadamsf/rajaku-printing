package model

import "github.com/google/uuid"

// DiscountProduct — satu baris = satu produk yang termasuk cakupan sebuah
// diskon applies_to='selected' (§28.9). Kalau applies_to='all', tabel ini
// tidak relevan sama sekali untuk diskon tersebut. Tidak ada soft delete di
// sini — beda dengan Discount sendiri — karena baris ini cuma daftar
// cakupan, bukan catatan transaksi (migration 000028: FK discount_id ON
// DELETE CASCADE, boleh cascade).
type DiscountProduct struct {
	DiscountID uuid.UUID `gorm:"type:uuid;primaryKey;column:discount_id"`
	ProductID  uuid.UUID `gorm:"type:uuid;primaryKey;column:product_id"`
}

func (DiscountProduct) TableName() string { return "discount_products" }
