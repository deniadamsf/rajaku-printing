// Package notificationapi is the PUBLIC contract of the notification module
// (spec §22 — cross-module import allowed hanya lewat package api). Modul lain
// (order, payment, design, production, invoice) hanya boleh import package ini
// untuk trigger notifikasi WA.
package notificationapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Sentinel errors — semua boleh error, tapi caller SEBAIKNYA log & continue
// (jangan bikin pembayaran gagal cuma karena WA gagal di-enqueue). Notification
// harus best-effort, tidak boleh block critical path.
var (
	ErrOrderNotFound    = errors.New("notificationapi: order not found for enqueue")
	ErrRecipientMissing = errors.New("notificationapi: recipient phone unresolved")
	// ErrInternalRecipientMissing — NOTIFICATION_INTERNAL_PHONE tidak di-set,
	// jadi alert internal (mis. reminder retensi §19) tidak punya tujuan.
	// Bukan fatal: caller log warn & lanjut — alert internal sifatnya bantuan
	// operasional, bukan jalur kritis.
	ErrInternalRecipientMissing = errors.New("notificationapi: internal alert phone not configured")
	ErrUnknownKind              = errors.New("notificationapi: unknown notification kind")
	ErrDuplicateEnqueue         = errors.New("notificationapi: notification already enqueued (dedup key exists)")
)

// Kind — enum-like string type. String literal biar mudah persist di DB
// tanpa migration setiap tambah trigger baru.
type Kind string

// Kind yang sudah punya template + trigger di modul consumer. Kind baru
// ditambah HANYA kalau modul consumer-nya (production, invoice, POS, design)
// sudah bikin trigger + template — jangan pre-declare kind yang tidak ada
// template-nya (§22 no-silent-stub).
const (
	KindOngkirReady         Kind = "ongkir_ready"          // total fix, minta bayar
	KindPaymentVerified     Kind = "payment_verified"      // bukti disetujui
	KindPaymentRejected     Kind = "payment_rejected"      // bukti ditolak, upload ulang
	KindDesignApproved      Kind = "design_approved"       // customer setuju draft desain (§6)
	KindDesignNeedsRevision Kind = "design_needs_revision" // customer minta revisi (§6)
	KindReadyPickup         Kind = "ready_pickup"          // pickup — siap diambil di toko
	KindReadyShip           Kind = "ready_ship"            // kirim  — siap dikirim (belum ada resi ekspedisi)
	KindShipped             Kind = "shipped"               // sudah dikirim, ada courier + tracking
	KindInvoiceReady        Kind = "invoice_ready"         // invoice PDF siap didownload (§12)
	KindPOSOrderCreated     Kind = "pos_order_created"     // konfirmasi walk-in order sudah diterima (§11)

	// KindDesignRetentionWarning — INTERNAL (ke nomor ops/staff, bukan
	// customer): file desain akan dihapus otomatis H-3 sementara order-nya
	// belum `selesai` (§19). Staff diberi kesempatan download manual dulu
	// kalau masih mungkin ada reprint.
	KindDesignRetentionWarning Kind = "design_retention_warning"

	// KindOTPVerification — kode verifikasi kepemilikan nomor WA, dipakai
	// modul auth (pendaftaran via Google OAuth). Tidak terikat order/customer
	// manapun (dikirim SEBELUM user row dibuat) — lihat OTPSender di bawah.
	KindOTPVerification Kind = "otp_verification"

	// Membership (§30 CLAUDE.md) — terikat CUSTOMER langsung, bukan order
	// (lihat CustomerEventEnqueuer di bawah).
	KindMembershipApproved   Kind = "membership_approved"   // pengajuan disetujui admin
	KindMembershipRejected   Kind = "membership_rejected"   // pengajuan ditolak admin, wajib alasan
	KindMembershipRevoked    Kind = "membership_revoked"    // status member dicabut admin, wajib alasan
	KindMembershipReinstated Kind = "membership_reinstated" // status member dipulihkan admin dari revoked, wajib alasan
)

// Enqueuer — kontrak untuk trigger notifikasi terkait order. Implementasi
// (notification/service.Service) resolve recipient phone otomatis dari
// order.customer + kirim.
//
// Kontrak untuk caller:
//   - Method ini NON-BLOCKING (INSERT ke DB saja). Boleh dipanggil sinkron
//     di dalam handler tanpa risiko latency.
//   - Kalau return error, caller SEBAIKNYA log level=error tapi TIDAK
//     rollback operasi bisnis (payment sudah verified, dll). Notifikasi
//     bisa dikirim manual dari admin panel nanti.
//   - Reason: WA gateway (Baileys) reliability ≠ 100%, ini best-effort.
type Enqueuer interface {
	// EnqueueOrderEvent renders template untuk `kind` dari order + extras,
	// resolve recipient phone (kirim → shipping_recipient_phone, pickup →
	// customer.phone), lalu insert row ke notification_jobs.
	//
	// extras — data tambahan template-specific:
	//   - ongkir_ready: {"shipping_cost": 15000}
	//   - payment_rejected: {"reason": "gambar buram"}
	//   - shipped: {"courier": "JNE", "tracking": "12345"}
	//
	// Idempotent lewat dedup_key = "<kind>:<orderID>" (untuk kind sekali-per-order).
	// Kalau caller memang mau kirim ulang (mis. reminder), pakai EnqueueOrderEventForce.
	EnqueueOrderEvent(ctx context.Context, kind Kind, orderID uuid.UUID, extras map[string]any) error
}

// JobCanceller — kontrak untuk modul lain (order) yang perlu membatalkan job
// notifikasi yang masih antre untuk sebuah order. Dipakai satu-satunya kasus
// saat ini: super admin soft-delete order (§ super admin order tools) — WA
// yang sudah di-enqueue (mis. "siap kirim") tapi belum terkirim TIDAK boleh
// tetap dikirim membawa link `/lacak/<resi>` yang sekarang 404.
type JobCanceller interface {
	// CancelOrderJobs marks every job with status pending/failed for orderID
	// as cancelled. Idempotent — no matching jobs returns (0, nil). Jobs
	// already sent/dead/cancelled are left untouched.
	CancelOrderJobs(ctx context.Context, orderID uuid.UUID) (int64, error)
}

// InternalAlerter — kontrak untuk notifikasi yang tujuannya TIM SENDIRI, bukan
// pelanggan (mis. reminder retensi file desain §19, atau alert operasional
// lain nanti). Recipient-nya satu nomor ops yang di-set lewat env
// NOTIFICATION_INTERNAL_PHONE — caller tidak perlu (dan tidak boleh) tahu
// nomornya, supaya tetap satu sumber konfigurasi (§2).
//
// Kontrak untuk caller — sama seperti Enqueuer: best-effort, jangan rollback
// operasi bisnis kalau gagal. Khususnya kalau return ErrInternalRecipientMissing,
// artinya fitur alert internal memang belum dikonfigurasi di deployment ini;
// log warn sekali, jangan retry.
type InternalAlerter interface {
	// EnqueueInternalAlert merender template `kind` lalu insert job dengan
	// tujuan nomor internal.
	//
	// orderID opsional — kalau di-set, service resolve resi & status order
	// untuk dipakai template + disimpan di payload (memudahkan telusur).
	//
	// dedupKey wajib diisi caller (mis. "design_retention_warning:<file_id>")
	// supaya reminder yang sama tidak dikirim dua kali saat job jalan ulang.
	EnqueueInternalAlert(ctx context.Context, kind Kind, orderID *uuid.UUID, extras map[string]any, dedupKey string) error
}

// CustomerEventEnqueuer — kontrak untuk trigger notifikasi yang terikat ke
// CUSTOMER langsung (bukan order) — dipakai modul membership (§30) untuk
// notif status approved/rejected/revoked, yang tidak melekat ke satu order
// tertentu (customer bisa punya banyak order, atau tidak punya sama sekali).
// Beda dari Enqueuer.EnqueueOrderEvent yang me-resolve recipient dari
// order.customer_id — di sini customerID diberikan langsung oleh caller.
//
// Kontrak untuk caller — sama seperti Enqueuer: NON-BLOCKING (INSERT ke DB
// saja), best-effort (jangan gagalkan operasi bisnis kalau enqueue gagal).
// dedupKey wajib diisi caller (§30 status membership bisa siklus — mis.
// active→revoked→pending→active lagi — jadi TIDAK bisa dedup statis per
// kind+customerID seperti EnqueueOrderEvent, yang mengasumsikan kind hanya
// terjadi sekali per order).
type CustomerEventEnqueuer interface {
	EnqueueCustomerEvent(ctx context.Context, kind Kind, customerID uuid.UUID, extras map[string]any, dedupKey string) error
}

// OTPSender — kontrak khusus untuk mengirim kode verifikasi kepemilikan
// nomor WA (OTP). Sengaja TERPISAH dari Enqueuer: Enqueuer selalu resolve
// recipient dari order.customer (butuh order & customer row yang sudah
// ada), sementara OTP dikirim SEBELUM user row dibuat sama sekali — caller
// (auth module, alur registrasi Google OAuth) sudah tahu persis nomor tujuan
// & pesan yang sudah dirender (kode + TTL), jadi tidak ada template/lookup
// yang perlu dilakukan modul ini.
//
// Kontrak untuk caller — sama seperti Enqueuer: NON-BLOCKING (INSERT saja),
// best-effort. dedupKey wajib diisi supaya retry yang salah tidak
// menggandakan pesan yang sama.
type OTPSender interface {
	EnqueueOTP(ctx context.Context, phone, message, dedupKey string) error
}
