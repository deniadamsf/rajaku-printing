package model

import "github.com/google/uuid"

// DiscountCustomer — satu baris = satu customer yang termasuk cakupan sebuah
// diskon audience_scope='member' & member_scope='selected_members' (§30.3).
// Kalau member_scope='all_members' (atau audience_scope != 'member' sama
// sekali), tabel ini tidak relevan untuk diskon tersebut. Tidak ada soft
// delete di sini — mirror discount_product.go — karena baris ini cuma
// daftar cakupan, bukan catatan transaksi (migration 000031: FK discount_id
// ON DELETE CASCADE, boleh cascade).
type DiscountCustomer struct {
	DiscountID uuid.UUID `gorm:"type:uuid;primaryKey;column:discount_id"`
	CustomerID uuid.UUID `gorm:"type:uuid;primaryKey;column:customer_id"`
}

func (DiscountCustomer) TableName() string { return "discount_customers" }
