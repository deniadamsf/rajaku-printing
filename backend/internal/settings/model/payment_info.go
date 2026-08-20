package model

// PaymentInfo — proyeksi PUBLIK dari 6 key payment.* di app_settings (§7).
//
// Sengaja struct terpisah dari AppSetting: endpoint publik GET /payment-info
// HANYA boleh membocorkan nilai rekening/QRIS, bukan seluruh isi tabel
// app_settings (yang bisa berisi setting internal lain di masa depan).
//
// Nama field JSON dikontrakkan ke frontend — jangan ubah tanpa koordinasi.
type PaymentInfo struct {
	BankName         string `json:"bank_name"`
	AccountName      string `json:"account_name"`
	AccountNumber    string `json:"account_number"`
	QRISNote         string `json:"qris_note"`
	QRISMerchantName string `json:"qris_merchant_name"`
	QRISNmid         string `json:"qris_nmid"`
}
