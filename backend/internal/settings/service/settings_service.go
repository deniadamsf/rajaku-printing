// Package service — business logic modul settings (§19: retensi configurable).
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/settings/model"
	settingsrepo "github.com/rajaku-printing/backend/internal/settings/repository"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// Store — narrowed repository contract untuk testability.
type Store interface {
	Get(ctx context.Context, key string) (*model.AppSetting, error)
	List(ctx context.Context) ([]model.AppSetting, error)
	Update(ctx context.Context, key, value string, actorID *uuid.UUID, at time.Time) error
}

// intRule — batas nilai yang masuk akal per key. Ini yang mencegah admin
// salah ketik "3000" jadi retensi 8 tahun, atau "0" yang bikin file kehapus
// seketika setelah upload.
//
// allowed — kalau non-nil, key ini bertipe ENUM (bukan rentang bebas): nilai
// WAJIB salah satu dari daftar ini, min/max diabaikan. Dipakai untuk key
// dengan pilihan tetap fisik, mis. lebar kertas thermal 58/80mm (§11/§12) —
// nilai di luar itu bukan cuma "kurang masuk akal", tapi memang tidak ada
// printer/roll-nya.
type intRule struct {
	min, max int
	allowed  []int
	unit     string
}

// intRules — registry key bertipe integer. Key yang tidak terdaftar di sini
// (dan tidak di rule tipe lain) ditolak Update dengan ErrUnknownKey — setting
// baru WAJIB didaftarkan bareng migration seed-nya (§22 no-silent-stub).
var intRules = map[string]intRule{
	settingsapi.KeyDesignRetentionDays: {min: 1, max: 365, unit: "hari"},
	settingsapi.KeyPOSReceiptWidthMM:   {allowed: settingsapi.POSReceiptWidthsMM, unit: "mm"},
}

// stringRule — batas & normalisasi nilai key bertipe string, mis. rekening
// pembayaran manual (§7). Melengkapi intRules di atas untuk key non-angka.
// Key string yang tidak terdaftar di sini (dan tidak di intRules) ditolak
// Update dengan ErrUnknownKey, sama seperti intRules.
type stringRule struct {
	// required — kalau true, string kosong ditolak. Dipakai untuk key yang
	// menentukan tujuan transfer (bank_name/account_name/account_number):
	// menyimpan nilai kosong berarti halaman pembayaran tampil tanpa tujuan.
	required bool
	maxLen   int
	// digitsOnly — kalau true, spasi & strip di input dibuang dulu, lalu
	// sisanya WAJIB semua digit. Nilai yang DISIMPAN adalah versi
	// ternormalisasi (digit saja), bukan input mentah — dipakai
	// payment.account_number supaya "3245 070-777" tersimpan "3245070777".
	digitsOnly bool
}

var stringRules = map[string]stringRule{
	settingsapi.KeyPaymentBankName:      {required: true, maxLen: 120},
	settingsapi.KeyPaymentAccountName:   {required: true, maxLen: 120},
	settingsapi.KeyPaymentAccountNumber: {required: true, maxLen: 120, digitsOnly: true},
	// QRIS boleh kosong — toko bisa saja belum punya QRIS.
	settingsapi.KeyPaymentQRISNote:         {maxLen: 120},
	settingsapi.KeyPaymentQRISMerchantName: {maxLen: 120},
	settingsapi.KeyPaymentQRISNmid:         {maxLen: 120},
}

type Service struct {
	store Store
	nowFn func() time.Time
}

var _ settingsapi.Reader = (*Service)(nil)

func New(store Store) *Service {
	return &Service{store: store, nowFn: time.Now}
}

// GetInt implements settingsapi.Reader.
//
// Sengaja TIDAK punya fallback diam-diam ke default: kalau row hilang, caller
// yang memutuskan (design retention job pakai nilai env sebagai fallback dan
// log warn). Menelan error di sini bikin misconfigurasi tidak kelihatan.
func (s *Service) GetInt(ctx context.Context, key string) (int, error) {
	row, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, settingsrepo.ErrNotFound) {
			return 0, settingsapi.ErrSettingNotFound
		}
		return 0, fmt.Errorf("settings: get %q: %w", key, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil {
		return 0, fmt.Errorf("settings: key %q value %q is not an integer: %w",
			key, row.Value, settingsapi.ErrInvalidValue)
	}
	return n, nil
}

// List mengembalikan semua setting untuk admin panel, masing-masing dilengkapi
// AllowedValues untuk key yang bertipe enum (intRule.allowed) — supaya
// frontend tidak perlu hardcode daftar pilihan yang gampang basi kalau
// backend menambah pilihan baru (mis. lebar kertas 76mm).
func (s *Service) List(ctx context.Context) ([]model.SettingItem, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	out := make([]model.SettingItem, len(items))
	for i, it := range items {
		out[i] = toSettingItem(it)
	}
	return out, nil
}

// toSettingItem memetakan entity AppSetting ke DTO transport SettingItem,
// mengisi AllowedValues dari intRules kalau key-nya terdaftar sebagai enum.
func toSettingItem(a model.AppSetting) model.SettingItem {
	item := model.SettingItem{
		Key:         a.Key,
		Value:       a.Value,
		DisplayName: a.DisplayName,
		Description: a.Description,
		UpdatedAt:   a.UpdatedAt,
		UpdatedBy:   a.UpdatedBy,
	}
	if rule, ok := intRules[a.Key]; ok && len(rule.allowed) > 0 {
		item.AllowedValues = formatAllowedInts(rule.allowed)
	}
	return item
}

// formatAllowedInts mengubah daftar int enum jadi daftar string, sebanding
// dengan Value yang juga string (§ handler updateRequest — value dikirim
// sebagai string apa adanya).
func formatAllowedInts(allowed []int) []string {
	out := make([]string, len(allowed))
	for i, a := range allowed {
		out[i] = strconv.Itoa(a)
	}
	return out
}

// Update memvalidasi lalu menyimpan nilai baru. Return row hasil update supaya
// handler bisa langsung balikin state terbaru ke UI.
func (s *Service) Update(ctx context.Context, key, rawValue string, actorID uuid.UUID) (*model.SettingItem, error) {
	key = strings.TrimSpace(key)
	value := strings.TrimSpace(rawValue)

	value, err := validateSettingValue(key, value)
	if err != nil {
		return nil, err
	}

	var actor *uuid.UUID
	if actorID != uuid.Nil {
		actor = &actorID
	}
	if err := s.store.Update(ctx, key, value, actor, s.nowFn().UTC()); err != nil {
		if errors.Is(err, settingsrepo.ErrNotFound) {
			return nil, settingsapi.ErrSettingNotFound
		}
		return nil, fmt.Errorf("settings: update %q: %w", key, err)
	}

	row, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("settings: reload %q after update: %w", key, err)
	}
	// Bentuk yang sama persis dengan List: frontend menimpa item di state
	// pakai hasil PUT ini. Kalau PUT balas AppSetting polos (tanpa
	// allowed_values), pilihan enum di UI hilang tepat setelah disimpan dan
	// baru muncul lagi setelah refresh halaman.
	item := toSettingItem(*row)
	return &item, nil
}

// validateSettingValue mencari rule key di intRules lalu stringRules (dalam
// urutan itu), lalu memvalidasi/menormalisasi. Return ErrUnknownKey kalau key
// tidak terdaftar di rule manapun — setting baru wajib didaftarkan bareng
// migration seed-nya (§22 no-silent-stub).
func validateSettingValue(key, value string) (string, error) {
	if rule, ok := intRules[key]; ok {
		return validateIntValue(key, value, rule)
	}
	if rule, ok := stringRules[key]; ok {
		return validateStringValue(key, value, rule)
	}
	return "", settingsapi.ErrUnknownKey
}

func validateIntValue(key, value string, rule intRule) (string, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", fmt.Errorf("settings: %q harus berupa angka, dapat %q: %w",
			key, value, settingsapi.ErrInvalidValue)
	}
	if len(rule.allowed) > 0 {
		return validateEnumIntValue(key, n, rule)
	}
	if n < rule.min || n > rule.max {
		return "", fmt.Errorf("settings: %q di luar rentang [%d, %d] %s, dapat %d: %w",
			key, rule.min, rule.max, rule.unit, n, settingsapi.ErrInvalidValue)
	}
	return strconv.Itoa(n), nil
}

// validateEnumIntValue mengecek keanggotaan n di rule.allowed. Dipisah dari
// validateIntValue supaya pesan errornya menyebut daftar pilihan yang valid
// (mis. "hanya boleh 58 atau 80 mm"), bukan rentang [min, max] yang tidak
// relevan untuk key enum.
func validateEnumIntValue(key string, n int, rule intRule) (string, error) {
	for _, a := range rule.allowed {
		if n == a {
			return strconv.Itoa(n), nil
		}
	}
	return "", fmt.Errorf("settings: %q hanya boleh %s %s, dapat %d: %w",
		key, joinAllowedInts(rule.allowed), rule.unit, n, settingsapi.ErrInvalidValue)
}

// joinAllowedInts merangkai daftar nilai yang diizinkan jadi teks natural
// bahasa Indonesia, mis. [58, 80] -> "58 atau 80".
func joinAllowedInts(allowed []int) string {
	parts := make([]string, len(allowed))
	for i, a := range allowed {
		parts[i] = strconv.Itoa(a)
	}
	return strings.Join(parts, " atau ")
}

// validateStringValue memvalidasi & menormalisasi nilai key string sesuai
// rule-nya. Return nilai yang WAJIB disimpan (bisa beda dari input mentah,
// mis. account_number yang di-strip spasi/strip).
func validateStringValue(key, value string, rule stringRule) (string, error) {
	if rule.digitsOnly {
		value = strings.NewReplacer(" ", "", "-", "").Replace(value)
		for _, r := range value {
			if r < '0' || r > '9' {
				return "", fmt.Errorf("settings: %q harus berupa angka saja, dapat %q: %w",
					key, value, settingsapi.ErrInvalidValue)
			}
		}
	}
	if rule.required && value == "" {
		return "", fmt.Errorf("settings: %q tidak boleh kosong: %w", key, settingsapi.ErrInvalidValue)
	}
	if len(value) > rule.maxLen {
		return "", fmt.Errorf("settings: %q maksimal %d karakter, dapat %d karakter: %w",
			key, rule.maxLen, len(value), settingsapi.ErrInvalidValue)
	}
	return value, nil
}

// GetPaymentInfo mengembalikan info rekening/QRIS publik (§7) yang dipetakan
// dari 6 key payment.* — HANYA enam key ini, bukan seluruh isi app_settings
// (endpoint publik tidak boleh membocorkan setting internal lain).
//
// Kalau salah satu row hilang (migration belum jalan / row terhapus manual),
// sengaja TIDAK fallback ke string kosong: caller (handler) harus tahu ini
// kegagalan konfigurasi, bukan "rekening memang dikosongkan admin" — kalau
// keduanya digabung, pelanggan tidak akan tahu harus transfer ke mana.
func (s *Service) GetPaymentInfo(ctx context.Context) (*model.PaymentInfo, error) {
	keys := []string{
		settingsapi.KeyPaymentBankName,
		settingsapi.KeyPaymentAccountName,
		settingsapi.KeyPaymentAccountNumber,
		settingsapi.KeyPaymentQRISNote,
		settingsapi.KeyPaymentQRISMerchantName,
		settingsapi.KeyPaymentQRISNmid,
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		row, err := s.store.Get(ctx, key)
		if err != nil {
			if errors.Is(err, settingsrepo.ErrNotFound) {
				return nil, fmt.Errorf(
					"settings: payment info: key %q hilang dari app_settings (migration 000020 belum jalan?)", key)
			}
			return nil, fmt.Errorf("settings: payment info: get %q: %w", key, err)
		}
		values[key] = row.Value
	}
	return &model.PaymentInfo{
		BankName:         values[settingsapi.KeyPaymentBankName],
		AccountName:      values[settingsapi.KeyPaymentAccountName],
		AccountNumber:    values[settingsapi.KeyPaymentAccountNumber],
		QRISNote:         values[settingsapi.KeyPaymentQRISNote],
		QRISMerchantName: values[settingsapi.KeyPaymentQRISMerchantName],
		QRISNmid:         values[settingsapi.KeyPaymentQRISNmid],
	}, nil
}
