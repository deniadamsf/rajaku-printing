// customer_admin_service.go — layar admin "Manajemen Pelanggan". Pelanggan
// secara fisik adalah baris `users` dengan user_type='customer' — TIDAK ada
// modul terpisah untuk ini (lihat internal/auth/model/user.go), pola sama
// persis staff_admin_service.go tapi untuk sisi customer.
//
// Aturan keamanan yang wajib dijaga di file ini (jangan dilanggar):
//  1. SEMUA query & update WAJIB menyaring user_type='customer' — kalau
//     tidak, pemegang permission customer.manage bisa mengedit/menonaktifkan
//     akun staff/super admin lewat endpoint pelanggan. Ini ditegakkan di
//     repository (UpdateCustomerBasic/SetCustomerActive/FindCustomerByID),
//     bukan cuma di sini — dua lapis, sama pola dengan validasi lain di
//     proyek ini.
//  2. PATCH pelanggan HANYA boleh mengubah name/email/phone — TIDAK PERNAH
//     password_hash, user_type, customer_type, roles, membership_status,
//     oauth_provider/subject.
//  3. Ganti nomor WA → phone_verified_at WAJIB ditulis NULL eksplisit (admin
//     mengetik nomor = "diklaim", bukan "terbukti dimiliki").
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/model"
	"github.com/rajaku-printing/backend/internal/auth/repository"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// --- Sentinel errors (§22 — errors.Is, bukan compare string) ---

var (
	// ErrInvalidCustomerFilter — filter customer_type/membership_status yang
	// dikirim bukan salah satu nilai resmi (pola sama
	// orderapi.ErrRecapInvalidStatus §28.5 — typo TIDAK boleh diam-diam
	// mengembalikan 0 baris).
	ErrInvalidCustomerFilter = errors.New("customer admin: filter tidak dikenal")
	// ErrPhoneTaken — nomor WA baru sudah dipakai user lain (customer/staff).
	ErrPhoneTaken = errors.New("customer admin: nomor WA sudah dipakai pelanggan/akun lain")
	// ErrEmailTaken — email baru sudah dipakai user lain.
	ErrEmailTaken = errors.New("customer admin: email sudah dipakai pelanggan/akun lain")
	// ErrCustomerNameRequired — nama tidak boleh dikosongkan (beda dari
	// email/phone yang boleh "" untuk clear — nama wajib selalu ada).
	ErrCustomerNameRequired = errors.New("customer admin: nama pelanggan wajib diisi")
	// ErrReasonRequired — alasan blokir/aktifkan kurang dari minReasonRuneLen
	// karakter.
	ErrReasonRequired = errors.New("customer admin: alasan wajib diisi, minimal 10 karakter")
	// ErrCustomerStatusUnchanged — mencoba mengaktifkan yang sudah aktif atau
	// menonaktifkan yang sudah nonaktif — ditolak supaya audit trail
	// (customer_admin_logs) tidak terisi baris no-op.
	ErrCustomerStatusUnchanged = errors.New("customer admin: status akun tidak berubah")
	// ErrExportTooLarge — hasil filter ekspor CSV melebihi maxExportRows.
	ErrExportTooLarge = errors.New("customer admin: hasil ekspor terlalu besar, persempit filter dulu")
	// ErrEmailRequiredForRegistered — customer_type='registered' TIDAK boleh
	// dikosongkan emailnya (CHECK users_email_required, migration 000001) —
	// email itulah kredensial loginnya. Ditolak DI SINI (400) sebelum sampai
	// ke DB, supaya admin dapat pesan yang jelas alih-alih 500 generic dari
	// pelanggaran CHECK constraint (temuan review #5).
	ErrEmailRequiredForRegistered = errors.New("customer admin: email wajib diisi untuk pelanggan terdaftar (registered)")
	// ErrPhoneRequiredForGuest — nomor WA adalah SATU-SATUNYA matching key
	// identitas guest (§11) dan SearchCustomers (layar kasir) menyaring
	// `phone IS NOT NULL` — mengosongkannya membuat pelanggan itu beserta
	// SELURUH riwayat order-nya tidak bisa ditemukan lagi di POS, tanpa jalan
	// balik (temuan review #5).
	ErrPhoneRequiredForGuest = errors.New("customer admin: nomor WA wajib diisi untuk pelanggan guest (matching key identitas)")
	// ErrCustomerUpdateInvalid — backstop SQLSTATE 23514 (CHECK constraint
	// violation) dari `users` yang lolos dari validasi di atas (mis. race
	// customer_type berubah konkuren) — dipetakan ke 400, BUKAN 500 (temuan
	// review #5).
	ErrCustomerUpdateInvalid = errors.New("customer admin: perubahan data melanggar aturan akun pelanggan")
)

const (
	minReasonRuneLen = 10
	// maxExportRows — batas keras ekspor CSV (§28.5 pola yang sama: batas
	// eksplisit dicek SEBELUM menulis apa pun, bukan dibiarkan tumbuh tanpa
	// batas di VPS Hostinger tunggal, §19).
	maxExportRows      = 50_000
	exportStreamBatch  = 1000
	defaultRecentLimit = 5
	defaultLogLimit    = 5
)

// --- Narrow store interfaces (§22 — unit-testable tanpa *gorm.DB) ---

// customerAdminUserStore narrows repository.UserRepository ke yang dibutuhkan
// CustomerAdminService — pola sama customerUserStore di customer_service.go.
type customerAdminUserStore interface {
	ListCustomers(ctx context.Context, f repository.ListCustomerFilter) (*repository.ListCustomerResult, error)
	StreamCustomers(ctx context.Context, f repository.ListCustomerFilter, batchSize int, fn func([]model.User) error) error
	FindCustomerByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UpdateCustomerBasic(ctx context.Context, id uuid.UUID, name string, email, phoneNum *string, phoneVerifiedAt *time.Time) error
	SetCustomerActive(ctx context.Context, id uuid.UUID, active bool) error
	ExistsByEmailExcluding(ctx context.Context, email string, excludeID uuid.UUID) (bool, error)
	ExistsByPhoneExcluding(ctx context.Context, phone string, excludeID uuid.UUID) (bool, error)
}

// customerAdminLogStore narrows repository.CustomerAdminLogRepository.
type customerAdminLogStore interface {
	Create(ctx context.Context, l *model.CustomerAdminLog) error
	ListByCustomer(ctx context.Context, customerID uuid.UUID, limit int) ([]repository.CustomerAdminLogView, error)
}

var (
	_ customerAdminUserStore = (*repository.UserRepository)(nil)
	_ customerAdminLogStore  = (*repository.CustomerAdminLogRepository)(nil)
)

// CustomerAdminService — orkestrator layar admin "Manajemen Pelanggan".
type CustomerAdminService struct {
	users  customerAdminUserStore
	logs   customerAdminLogStore
	orders orderapi.CustomerOrderReader
	// tx — commits a user write (status or profile) atomically WITH the
	// customer_admin_logs row describing it (temuan review #7/#9). `users`/
	// `logs` above remain the non-transactional stores for read paths
	// (ListCustomers/GetCustomer/ExportCustomers) that never need atomicity.
	tx customerAdminTxRunner
}

// NewCustomerAdminService — `users`, `logs`, dan `orders` WAJIB non-nil.
//
// BATAS pengecekan nil ini harus disadari jujur (temuan review #10): `users`
// dan `logs` adalah TIPE POINTER KONKRET (*repository.UserRepository,
// *repository.CustomerAdminLogRepository) — perbandingan `== nil` di sini
// akurat 100% untuk keduanya, tidak ada celah. `orders` BEDA — ia bertipe
// INTERFACE (orderapi.CustomerOrderReader), dan `orders == nil` HANYA
// menangkap interface yang benar-benar kosong. Ia TIDAK menangkap
// typed-nil-wrapped-in-interface (mis. `var s *orderservice.Service` yang
// belum di-assign lalu dioper sebagai orderapi.CustomerOrderReader — nilai
// interface-nya sendiri TIDAK nil, hanya isinya, sehingga cek ini lolos dan
// baru meledak sebagai nil-pointer saat admin membuka detail pelanggan di
// produksi, bukan saat startup seperti yang dijanjikan komentar ini
// sebelumnya). Nil (atau typed-nil) di sini SELALU berarti bug wiring
// composition-root (router.go), bukan kondisi runtime yang boleh ditangani
// dengan anggun — makanya panic saat startup, bukan error biasa (§22: panic
// hanya untuk config/wiring invalid saat startup).
func NewCustomerAdminService(users *repository.UserRepository, logs *repository.CustomerAdminLogRepository, orders orderapi.CustomerOrderReader, db *gorm.DB) *CustomerAdminService {
	if users == nil {
		panic("customer admin service: *repository.UserRepository wajib di-wire (composition-root bug) — lihat NewCustomerAdminService doc")
	}
	if logs == nil {
		panic("customer admin service: *repository.CustomerAdminLogRepository wajib di-wire (composition-root bug) — lihat NewCustomerAdminService doc")
	}
	if orders == nil {
		panic("customer admin service: orderapi.CustomerOrderReader wajib di-wire (composition-root bug) — lihat NewCustomerAdminService doc")
	}
	if db == nil {
		panic("customer admin service: *gorm.DB wajib di-wire (composition-root bug) — lihat NewCustomerAdminService doc")
	}
	return &CustomerAdminService{users: users, logs: logs, orders: orders, tx: newGormCustomerAdminTxRunner(db)}
}

// --- DTO ---

// ListCustomerInput — GET /admin/customers query params.
type ListCustomerInput struct {
	Q                string
	CustomerType     string
	IsActive         *bool
	MembershipStatus string
	Page             int
	PerPage          int
}

// UpdateCustomerInput — PATCH /admin/customers/:id. Pointer nil = tidak
// diubah; string kosong ("") pada Email/Phone = kosongkan kolom itu. Name
// BUKAN pointer nullable secara bisnis (selalu wajib ada isi), tapi tetap
// pointer di DTO supaya PATCH bisa membedakan "key absen di body" (nil,
// tidak diubah) dari "key dikirim" (divalidasi non-kosong).
type UpdateCustomerInput struct {
	CustomerID uuid.UUID
	Name       *string
	Email      *string
	Phone      *string
	// ActorID — staff yang melakukan PATCH ini, dicatat sebagai ChangedBy di
	// customer_admin_logs (action=profile_update, temuan review #9).
	ActorID uuid.UUID
}

// SetActiveInput — POST /admin/customers/:id/{deactivate,activate}.
type SetActiveInput struct {
	CustomerID uuid.UUID
	Active     bool
	Reason     string
	ActorID    uuid.UUID
}

// CustomerOutput — representasi JSON pelanggan (dipakai list, detail, dan
// respons mutasi) — bentuk kontrak PERSIS seperti yang dikonsumsi frontend,
// lihat brief fitur ini.
type CustomerOutput struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Phone            *string    `json:"phone,omitempty"`
	PhoneVerified    bool       `json:"phone_verified"`
	Email            *string    `json:"email,omitempty"`
	CustomerType     string     `json:"customer_type"`
	IsActive         bool       `json:"is_active"`
	MembershipStatus string     `json:"membership_status"`
	OAuthProvider    *string    `json:"oauth_provider,omitempty"`
	LastLoginAt      *time.Time `json:"last_login_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ListCustomerOutput — GET /admin/customers response body.
type ListCustomerOutput struct {
	Items   []CustomerOutput `json:"items"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}

// OrderStatsOutput mirrors orderapi.CustomerOrderStats, JSON-tagged.
type OrderStatsOutput struct {
	TotalOrders     int64      `json:"total_orders"`
	CompletedOrders int64      `json:"completed_orders"`
	CancelledOrders int64      `json:"cancelled_orders"`
	TotalSpend      int64      `json:"total_spend"`
	LastOrderAt     *time.Time `json:"last_order_at,omitempty"`
}

// RecentOrderOutput mirrors orderapi.CustomerOrderBrief, JSON-tagged.
type RecentOrderOutput struct {
	Resi      string    `json:"resi"`
	Status    string    `json:"status"`
	Channel   string    `json:"channel"`
	Total     int64     `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminLogOutput mirrors repository.CustomerAdminLogView, JSON-tagged.
// Kontrak PERSIS (kunci "admin_logs", bukan "status_logs") — lihat brief
// fitur ini soal perubahan kontrak JSON, frontend disesuaikan paralel.
type AdminLogOutput struct {
	Action        string    `json:"action"`
	Reason        *string   `json:"reason,omitempty"`
	Changes       *string   `json:"changes,omitempty"`
	ChangedByName *string   `json:"changed_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CustomerDetailOutput — GET /admin/customers/:id response body.
type CustomerDetailOutput struct {
	Customer     CustomerOutput      `json:"customer"`
	OrderStats   OrderStatsOutput    `json:"order_stats"`
	RecentOrders []RecentOrderOutput `json:"recent_orders"`
	AdminLogs    []AdminLogOutput    `json:"admin_logs"`
}

// --- ListCustomers ---

// ListCustomers implements GET /admin/customers.
func (s *CustomerAdminService) ListCustomers(ctx context.Context, in ListCustomerInput) (*ListCustomerOutput, error) {
	if err := validateCustomerFilterValues(in.CustomerType, in.MembershipStatus); err != nil {
		return nil, err
	}
	res, err := s.users.ListCustomers(ctx, repository.ListCustomerFilter{
		Q:                in.Q,
		CustomerType:     in.CustomerType,
		IsActive:         in.IsActive,
		MembershipStatus: in.MembershipStatus,
		Page:             in.Page,
		PageSize:         in.PerPage,
	})
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	items := make([]CustomerOutput, len(res.Items))
	for i := range res.Items {
		items[i] = toCustomerOutput(&res.Items[i])
	}
	return &ListCustomerOutput{Items: items, Total: res.Total, Page: res.Page, PerPage: res.PageSize}, nil
}

// GetCustomer implements GET /admin/customers/:id — profil + statistik order
// + riwayat blokir/aktifkan. Kalau pemanggilan orderapi gagal, error
// dikembalikan apa adanya (dibungkus konteks) — TIDAK disembunyikan sebagai
// statistik kosong (§22 no-silent-stub).
func (s *CustomerAdminService) GetCustomer(ctx context.Context, id uuid.UUID) (*CustomerDetailOutput, error) {
	u, err := s.users.FindCustomerByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrCustomerNotFound
		}
		return nil, fmt.Errorf("get customer %s: %w", id, err)
	}

	overview, err := s.orders.CustomerOrderOverview(ctx, id, defaultRecentLimit)
	if err != nil {
		return nil, fmt.Errorf("get customer %s: order overview: %w", id, err)
	}
	logRows, err := s.logs.ListByCustomer(ctx, id, defaultLogLimit)
	if err != nil {
		return nil, fmt.Errorf("get customer %s: admin logs: %w", id, err)
	}

	return &CustomerDetailOutput{
		Customer:     toCustomerOutput(u),
		OrderStats:   toOrderStatsOutput(overview),
		RecentOrders: toRecentOrderOutputs(overview),
		AdminLogs:    toAdminLogOutputs(logRows),
	}, nil
}

// UpdateCustomer implements PATCH /admin/customers/:id.
func (s *CustomerAdminService) UpdateCustomer(ctx context.Context, in UpdateCustomerInput) (*CustomerOutput, error) {
	existing, err := s.users.FindCustomerByID(ctx, in.CustomerID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrCustomerNotFound
		}
		return nil, fmt.Errorf("update customer %s: load: %w", in.CustomerID, err)
	}

	name, err := resolveCustomerName(existing.Name, in.Name)
	if err != nil {
		return nil, err
	}
	finalEmail, err := s.resolveCustomerEmail(ctx, existing, in.Email)
	if err != nil {
		return nil, err
	}
	finalPhone, phoneChanged, err := s.resolveCustomerPhone(ctx, existing, in.Phone)
	if err != nil {
		return nil, err
	}

	// § aturan keamanan #3 — nomor berubah (termasuk dikosongkan) => stempel
	// verifikasi lama TIDAK boleh menempel di keadaan baru.
	phoneVerifiedAt := existing.PhoneVerifiedAt
	if phoneChanged {
		phoneVerifiedAt = nil
	}

	// changes — dihitung SEBELUM tx dibuka (butuh nilai `existing` pra-update).
	// "" berarti PATCH ini tidak benar-benar mengubah name/email/phone apa
	// pun (mis. admin submit ulang nilai yang sama) — TIDAK menulis baris
	// audit log sama sekali (temuan review #9), sama filosofinya dengan
	// ErrCustomerStatusUnchanged di SetCustomerActive: no-op tidak layak
	// baris jejak.
	changes := buildCustomerProfileChangeSummary(existing, name, finalEmail, finalPhone)

	// temuan review #7/#9 — tulis field customer + baris audit log dalam SATU
	// transaksi: kalau insert audit log gagal, perubahan field HARUS ikut
	// batal, bukan tersimpan tanpa jejak siapa/kenapa yang mengubahnya.
	txErr := s.tx.RunInTx(ctx, func(tx customerAdminTx) error {
		if err := tx.Users.UpdateCustomerBasic(ctx, in.CustomerID, name, finalEmail, finalPhone, phoneVerifiedAt); err != nil {
			return err
		}
		if changes == "" {
			return nil
		}
		actorID := in.ActorID
		changesCopy := changes
		if err := tx.Logs.Create(ctx, &model.CustomerAdminLog{
			CustomerID: in.CustomerID,
			Action:     model.CustomerAdminActionProfileUpdate,
			Changes:    &changesCopy,
			ChangedBy:  &actorID,
		}); err != nil {
			return fmt.Errorf("write profile update audit log: %w", err)
		}
		return nil
	})
	if txErr != nil {
		switch {
		case errors.Is(txErr, repository.ErrNotFound):
			return nil, authapi.ErrCustomerNotFound
		case errors.Is(txErr, repository.ErrCustomerPhoneConflict):
			return nil, ErrPhoneTaken
		case errors.Is(txErr, repository.ErrCustomerEmailConflict):
			return nil, ErrEmailTaken
		case errors.Is(txErr, repository.ErrCustomerCheckViolation):
			return nil, ErrCustomerUpdateInvalid
		default:
			return nil, fmt.Errorf("update customer %s: %w", in.CustomerID, txErr)
		}
	}

	// Re-read supaya response mencerminkan baris yang benar-benar tersimpan
	// (pola sama service lain yang butuh nilai pasca-update, mis. order
	// SetShippingCost).
	updated, err := s.users.FindCustomerByID(ctx, in.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("update customer %s: reload: %w", in.CustomerID, err)
	}
	out := toCustomerOutput(updated)
	return &out, nil
}

// buildCustomerProfileChangeSummary renders a human-readable OLD -> NEW
// summary of the fields UpdateCustomer actually changed, for
// customer_admin_logs.changes (temuan review #9). Old values are included ON
// PURPOSE — without them the trail is useless for tracing a mis-attached
// walk-in order back to whichever number it used to be. Returns "" when
// name/email/phone all stayed the same (caller skips writing a log row).
func buildCustomerProfileChangeSummary(existing *model.User, newName string, newEmail, newPhone *string) string {
	var parts []string
	if existing.Name != newName {
		parts = append(parts, fmt.Sprintf("nama: %q → %q", existing.Name, newName))
	}
	if oldEmail, newEmailStr := derefOrEmpty(existing.Email), derefOrEmpty(newEmail); oldEmail != newEmailStr {
		parts = append(parts, fmt.Sprintf("email: %q → %q", oldEmail, newEmailStr))
	}
	if oldPhone, newPhoneStr := derefOrEmpty(existing.Phone), derefOrEmpty(newPhone); oldPhone != newPhoneStr {
		parts = append(parts, fmt.Sprintf("wa: %q → %q", oldPhone, newPhoneStr))
	}
	return strings.Join(parts, "; ")
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// resolveCustomerName applies the "nil = unchanged, non-nil must be
// non-empty" rule for the required Name field.
func resolveCustomerName(current string, in *string) (string, error) {
	if in == nil {
		return current, nil
	}
	trimmed := strings.TrimSpace(*in)
	if trimmed == "" {
		return "", ErrCustomerNameRequired
	}
	return trimmed, nil
}

// resolveCustomerEmail applies "nil = unchanged, "" = clear, else set (after
// dedup check)" and returns the FINAL pointer value UpdateCustomerBasic
// should write (repository semantics: pointer nil = write NULL).
func (s *CustomerAdminService) resolveCustomerEmail(ctx context.Context, existing *model.User, in *string) (*string, error) {
	if in == nil {
		return existing.Email, nil
	}
	// Lowercase — SEMUA jalur lain (auth_service.go, google_oauth_service.go)
	// menyimpan & mencari email lowercase, dan FindByEmail membandingkan
	// persis (bukan case-insensitive). Tanpa ini, admin yang mengetik
	// "Budi@Gmail.com" membuat pelanggan itu tidak bisa login lagi dengan
	// emailnya sendiri (temuan review #4).
	trimmed := strings.ToLower(strings.TrimSpace(*in))
	if trimmed == "" {
		if existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeRegistered {
			return nil, ErrEmailRequiredForRegistered
		}
		return nil, nil
	}
	taken, err := s.users.ExistsByEmailExcluding(ctx, trimmed, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("check email uniqueness: %w", err)
	}
	if taken {
		return nil, ErrEmailTaken
	}
	return &trimmed, nil
}

// resolveCustomerPhone is resolveCustomerEmail's phone counterpart, plus it
// normalizes to 62xxx (§13) and reports whether the FINAL value differs from
// the customer's current phone — the caller needs that to decide whether
// phone_verified_at must be reset to NULL (§ aturan keamanan #3).
func (s *CustomerAdminService) resolveCustomerPhone(ctx context.Context, existing *model.User, in *string) (*string, bool, error) {
	current := existing.Phone
	if in == nil {
		return current, false, nil
	}
	trimmed := strings.TrimSpace(*in)
	if trimmed == "" {
		if existing.CustomerType != nil && *existing.CustomerType == model.CustomerTypeGuest {
			return nil, false, ErrPhoneRequiredForGuest
		}
		changed := current != nil
		return nil, changed, nil
	}
	normalized, err := phone.Normalize(trimmed)
	if err != nil {
		return nil, false, fmt.Errorf("phone: %w", err)
	}
	taken, err := s.users.ExistsByPhoneExcluding(ctx, normalized, existing.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check phone uniqueness: %w", err)
	}
	if taken {
		return nil, false, ErrPhoneTaken
	}
	changed := current == nil || *current != normalized
	return &normalized, changed, nil
}

// SetCustomerActive implements POST /admin/customers/:id/{deactivate,activate}.
//
// BATAS YANG JUJUR HARUS DISADARI (temuan review #2): VerifyToken
// (auth_service.go) itu STATELESS — ia memvalidasi tanda tangan JWT dan
// tanggal kedaluwarsanya saja, TIDAK query DB. Jadi menonaktifkan pelanggan
// di sini TIDAK mencabut access token yang SUDAH terbit ke pelanggan itu —
// token tersebut tetap sah sampai TTL-nya habis (maksimum 24 jam, lihat
// token/jwt.go). Yang benar-benar ditegakkan oleh is_active=false hanyalah:
// (a) login BERIKUTNYA ditolak, dan (b) order BARU ditolak
// (authapi.ErrCustomerBlocked, lihat ResolveOrCreateGuest di
// customer_service.go) — BUKAN pencabutan sesi yang sedang berjalan. Jangan
// klaim lebih dari itu ke admin (§22 — no-silent-overclaim).
func (s *CustomerAdminService) SetCustomerActive(ctx context.Context, in SetActiveInput) (*CustomerOutput, error) {
	if utf8.RuneCountInString(strings.TrimSpace(in.Reason)) < minReasonRuneLen {
		return nil, ErrReasonRequired
	}

	existing, err := s.users.FindCustomerByID(ctx, in.CustomerID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, authapi.ErrCustomerNotFound
		}
		return nil, fmt.Errorf("set customer active %s: load: %w", in.CustomerID, err)
	}
	if existing.IsActive == in.Active {
		return nil, ErrCustomerStatusUnchanged
	}

	actorID := in.ActorID
	reason := strings.TrimSpace(in.Reason)
	action := model.CustomerAdminActionUnblock
	if !in.Active {
		action = model.CustomerAdminActionBlock
	}

	// temuan review #7 — status + baris audit log dalam SATU transaksi: kalau
	// insert audit gagal, is_active HARUS ikut batal, bukan tersimpan
	// terblokir/aktif TANPA baris alasan di audit trail.
	txErr := s.tx.RunInTx(ctx, func(tx customerAdminTx) error {
		if err := tx.Users.SetCustomerActive(ctx, in.CustomerID, in.Active); err != nil {
			return err
		}
		if err := tx.Logs.Create(ctx, &model.CustomerAdminLog{
			CustomerID: in.CustomerID,
			Action:     action,
			Reason:     &reason,
			ChangedBy:  &actorID,
		}); err != nil {
			return fmt.Errorf("write audit log: %w", err)
		}
		return nil
	})
	if txErr != nil {
		if errors.Is(txErr, repository.ErrNotFound) {
			return nil, authapi.ErrCustomerNotFound
		}
		return nil, fmt.Errorf("set customer active %s: %w", in.CustomerID, txErr)
	}

	existing.IsActive = in.Active
	out := toCustomerOutput(existing)
	return &out, nil
}

// ExportCustomers implements GET /admin/customers-export — CSV mengalir per
// batch (§28.5 pola streaming), batas keras maxExportRows dicek SEBELUM
// menulis apa pun. Statistik order per baris SENGAJA TIDAK disertakan — itu
// berarti satu query orderapi PER pelanggan yang diekspor, melanggar §22
// (order module hanya boleh diakses via kontrak batch/agregat, bukan N+1
// per-row) dan akan meledak jumlah query untuk ekspor besar.
func (s *CustomerAdminService) ExportCustomers(ctx context.Context, in ListCustomerInput, w io.Writer) error {
	if err := validateCustomerFilterValues(in.CustomerType, in.MembershipStatus); err != nil {
		return err
	}
	filter := repository.ListCustomerFilter{
		Q:                in.Q,
		CustomerType:     in.CustomerType,
		IsActive:         in.IsActive,
		MembershipStatus: in.MembershipStatus,
	}

	// Hitung dulu (Page=1,PageSize=1 cukup — kita cuma butuh Total) supaya
	// batas maxExportRows ditegakkan SEBELUM baris pertama ditulis (§28.5
	// pola yang sama: validasi selalu terjadi sebelum byte pertama keluar).
	countRes, err := s.users.ListCustomers(ctx, repository.ListCustomerFilter{
		Q: filter.Q, CustomerType: filter.CustomerType, IsActive: filter.IsActive,
		MembershipStatus: filter.MembershipStatus, Page: 1, PageSize: 1,
	})
	if err != nil {
		return fmt.Errorf("export customers: count: %w", err)
	}
	if countRes.Total > maxExportRows {
		return ErrExportTooLarge
	}

	csvWriter := newCustomerCSVWriter(w)
	if err := csvWriter.writeHeader(); err != nil {
		return fmt.Errorf("export customers: write header: %w", err)
	}

	streamErr := s.users.StreamCustomers(ctx, filter, exportStreamBatch, func(batch []model.User) error {
		for i := range batch {
			if err := csvWriter.writeRow(&batch[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if streamErr != nil {
		return fmt.Errorf("export customers: stream: %w", streamErr)
	}
	csvWriter.flush()
	return csvWriter.err()
}

// --- filter validation ---

func validateCustomerFilterValues(customerType, membershipStatus string) error {
	switch customerType {
	case "", string(model.CustomerTypeGuest), string(model.CustomerTypeRegistered):
	default:
		return ErrInvalidCustomerFilter
	}
	switch model.MembershipStatus(membershipStatus) {
	case "", model.MembershipStatusNone, model.MembershipStatusPending,
		model.MembershipStatusActive, model.MembershipStatusRejected, model.MembershipStatusRevoked:
	default:
		return ErrInvalidCustomerFilter
	}
	return nil
}

// --- view mapping ---

func toCustomerOutput(u *model.User) CustomerOutput {
	out := CustomerOutput{
		ID:               u.ID,
		Name:             u.Name,
		Phone:            u.Phone,
		PhoneVerified:    u.PhoneVerifiedAt != nil,
		Email:            u.Email,
		IsActive:         u.IsActive,
		MembershipStatus: string(u.MembershipStatus),
		OAuthProvider:    u.OAuthProvider,
		LastLoginAt:      u.LastLoginAt,
		CreatedAt:        u.CreatedAt,
	}
	if u.CustomerType != nil {
		out.CustomerType = string(*u.CustomerType)
	}
	return out
}

func toOrderStatsOutput(o *orderapi.CustomerOrderOverview) OrderStatsOutput {
	return OrderStatsOutput{
		TotalOrders:     o.Stats.TotalOrders,
		CompletedOrders: o.Stats.CompletedOrders,
		CancelledOrders: o.Stats.CancelledOrders,
		TotalSpend:      o.Stats.TotalSpend,
		LastOrderAt:     o.Stats.LastOrderAt,
	}
}

func toRecentOrderOutputs(o *orderapi.CustomerOrderOverview) []RecentOrderOutput {
	out := make([]RecentOrderOutput, 0, len(o.Recent))
	for _, r := range o.Recent {
		out = append(out, RecentOrderOutput{
			Resi: r.Resi, Status: r.Status, Channel: r.Channel, Total: r.Total, CreatedAt: r.CreatedAt,
		})
	}
	return out
}

func toAdminLogOutputs(rows []repository.CustomerAdminLogView) []AdminLogOutput {
	out := make([]AdminLogOutput, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminLogOutput{
			Action: r.Action, Reason: r.Reason, Changes: r.Changes,
			ChangedByName: r.ChangedByName, CreatedAt: r.CreatedAt,
		})
	}
	return out
}
