// Package orderapi is the PUBLIC contract of the order module — only this
// package boleh diimport oleh modul lain (payment, design, production nanti).
package orderapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrOrderNotFound          = errors.New("orderapi: order not found")
	ErrNotOwner               = errors.New("orderapi: caller is not the order owner")
	ErrInvalidTransition      = errors.New("orderapi: invalid state transition")
	ErrResiCollisionGaveUp    = errors.New("orderapi: could not generate unique resi after retries")
	ErrShippingFieldsRequired = errors.New("orderapi: shipping address/recipient/phone required when metode_ambil=kirim")
	ErrPickupNoShipping       = errors.New("orderapi: pickup order must not carry shipping_cost")
	// ErrShippingCostOnPickup — admin tried to set ongkir on a pickup order.
	ErrShippingCostOnPickup = errors.New("orderapi: shipping_cost cannot be set on pickup order")
	// ErrConfirmPickupOnKirim — admin tried to confirm-pickup a kirim order.
	ErrConfirmPickupOnKirim = errors.New("orderapi: confirm-pickup only valid for metode_ambil=pickup")
	// ErrOrderStateChanged — the order moved to a different status between
	// read and update (concurrent staff action). UI should refresh & retry.
	ErrOrderStateChanged = errors.New("orderapi: order state changed concurrently")
	// ErrInvalidShippingCost — negative or absurd ongkir value.
	ErrInvalidShippingCost = errors.New("orderapi: shipping_cost must be >= 0")
	// ErrPaymentAlreadySettled — attempt to submit payment proof for an order
	// already in dibayar / selesai / dibatalkan (nothing to do).
	ErrPaymentAlreadySettled = errors.New("orderapi: order already settled")

	// ErrDikirimOnPickup — attempt to mark shipped on a pickup order.
	ErrDikirimOnPickup = errors.New("orderapi: mark-shipped only valid for metode_ambil=kirim")
	// ErrShippingTrackingRequired — courier/tracking wajib saat mark dikirim.
	ErrShippingTrackingRequired = errors.New("orderapi: courier & tracking number required when marking shipped")
)

// OrderSummary — projection modul lain (payment, notification, POS) butuh baca
// order tanpa import order/model. Field yang di-expose sengaja dibatasi.
type OrderSummary struct {
	ID                 uuid.UUID
	Resi               string
	CustomerID         uuid.UUID
	Status             string
	Total              int64
	MetodeAmbil        string
	MetodeBayar        string // "" kalau belum settled
	Channel            string
	DesignSource       string     // "upload" | "request" (§6)
	DesignApprovalMode *string    // "instant_walkin" | "async_notify" (§11); nil kalau belum di-set
	CreatedBy          *uuid.UUID // kasir POS (nil untuk order online)
	CreatedAt          time.Time
}

// POSCreateOrderInput — payload untuk create walk-in order (§11).
// Customer sudah harus resolved (nomor WA → CustomerID via authapi.CustomerService).
// Metode bayar wajib POS-specific: cash atau qris_pos.
type POSCreateOrderInput struct {
	CustomerID uuid.UUID
	KasirID    uuid.UUID // staff yg input order (untuk rekonsiliasi)

	// Product spec — service akan Quote via catalog untuk hitung harga
	// otoritatif (jangan trust harga dari client).
	ProductID  uuid.UUID
	MaterialID uuid.UUID
	WidthCm    int
	HeightCm   int
	Quantity   int

	// Fulfillment
	MetodeAmbil            string // "pickup" | "kirim"
	ShippingAddress        string // wajib kalau kirim
	ShippingRecipientName  string
	ShippingRecipientPhone string // raw, service normalize
	ShippingCost           int64  // POS: kasir hitung langsung, tidak async

	// Payment (POS-only)
	MetodeBayar string // "cash" | "qris_pos"

	// Design
	DesignSource       string // "upload" | "request"
	DesignApprovalMode string // "instant_walkin" | "async_notify" (§11); "" = tidak di-set
	DesignBrief        string

	Notes string
}

// OrderInvoiceView — projection lebih lengkap khusus modul invoice: mencakup
// line item snapshot, alamat, dan total. Tetap terbatas — jangan expose
// internal seperti design_source atau reject reason.
type OrderInvoiceView struct {
	ID          uuid.UUID
	Resi        string
	CustomerID  uuid.UUID
	CreatedAt   time.Time
	Channel     string
	Status      string
	MetodeAmbil string
	MetodeBayar string // "" kalau belum settled

	// Line item — MVP satu produk per order.
	ProductName  string
	MaterialName string
	WidthCm      int
	HeightCm     int
	Quantity     int
	UnitPrice    int64
	Subtotal     int64

	// Fulfillment
	ShippingCost       *int64
	ShippingAddress    *string
	ShippingRecipient  *string
	ShippingPhone      *string
	ShippingCourier    *string
	ShippingTrackingNo *string

	Total int64
	Notes *string
}

// OrderCommandService — kontrak untuk modul lain yang perlu memerintahkan
// transisi status order. Order module mengontrol validasi state machine di
// implementasinya — modul consumer TIDAK boleh punya pengetahuan state machine.
//
// actorID nullable — nil kalau system-triggered (mis. cron/job). Kalau customer
// atau staff, isi UserID untuk audit trail (state_history.changed_by).
type OrderCommandService interface {
	// FindSummaryByResi returns a projection of the order — used by consumer
	// modules to guard against acting on nonexistent or already-settled orders.
	FindSummaryByResi(ctx context.Context, resi string) (*OrderSummary, error)

	// FindSummaryByID returns the projection by primary key. Used when a
	// consumer holds only an order_id foreign key (e.g. payment_proofs.order_id).
	FindSummaryByID(ctx context.Context, id uuid.UUID) (*OrderSummary, error)

	// FindInvoiceViewByID returns the richer OrderInvoiceView. Dipakai modul
	// invoice untuk render PDF (line items + alamat + total).
	FindInvoiceViewByID(ctx context.Context, id uuid.UUID) (*OrderInvoiceView, error)

	// MarkPendingVerification advances the order to menunggu_verifikasi.
	// Valid from menunggu_pembayaran (first upload) or ditolak (re-upload).
	MarkPendingVerification(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDibayar advances the order to dibayar and records metode_bayar.
	// Valid from menunggu_verifikasi (staff approves proof) or menunggu_pembayaran
	// (POS cash/qris — no proof needed).
	MarkDibayar(ctx context.Context, orderID uuid.UUID, metodeBayar string, actorID *uuid.UUID, note string) error

	// MarkDitolak advances the order to ditolak (staff rejected the proof).
	// Valid only from menunggu_verifikasi. Reason is required, stored in the
	// state_history note.
	MarkDitolak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, reason string) error

	// --- Design flow transitions (§6, §11) ---

	// MarkDesainDikerjakan — staff mulai/lanjut kerjakan desain.
	// Valid dari: dibayar (initial) atau menunggu_approval_desain (revisi).
	MarkDesainDikerjakan(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkMenungguApprovalDesain — staff selesai draft, kirim ke customer.
	// Valid dari: desain_dikerjakan.
	MarkMenungguApprovalDesain(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDesainDiverifikasi — desain final, siap masuk cetak.
	// Valid dari:
	//   - dibayar               (upload path: customer upload file valid & staff verifikasi)
	//   - menunggu_approval_desain (request path: customer approve draft)
	//   - desain_dikerjakan     (walk-in POS: "Disetujui Langsung" §11)
	MarkDesainDiverifikasi(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// --- Production flow transitions (§3.7, §4) ---

	// MarkProsesCetak — staff mulai cetak. Valid dari desain_diverifikasi.
	MarkProsesCetak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkQC — hasil cetak masuk QC. Valid dari proses_cetak.
	MarkQC(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkSiapKirimAtauAmbil — lulus QC, siap diserahkan.
	// Auto-branch berdasarkan order.metode_ambil:
	//   - kirim  → siap_kirim
	//   - pickup → siap_ambil
	// Valid dari qc.
	MarkSiapKirimAtauAmbil(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// MarkDikirim — paket diserahkan ke ekspedisi. Kurir & no. resi ekspedisi
	// direcord di order (ShippingCourier, ShippingTrackingNumber).
	// Valid dari siap_kirim. Wajib metode_ambil=kirim.
	MarkDikirim(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, courier, trackingNumber, note string) error

	// MarkSelesai — order selesai. Valid dari:
	//   - dikirim    (kirim path)
	//   - siap_ambil (pickup path — customer sudah ambil)
	MarkSelesai(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error

	// --- POS / walk-in (§11) ---

	// CreatePOSOrder atomically creates walk-in order langsung di status
	// dibayar (skip menunggu_pembayaran & menunggu_verifikasi — cash/qris
	// di tempat, tidak butuh verifikasi async). Metode_bayar direcord.
	// Initial history row: from=null, to=dibayar (POS instant payment).
	CreatePOSOrder(ctx context.Context, in POSCreateOrderInput) (*OrderSummary, error)

	// ListPOSOrdersByDate returns all POS orders created on the given date
	// (WIB timezone). Dipakai modul POS untuk rekonsiliasi harian.
	// Result ordered by created_at ASC.
	ListPOSOrdersByDate(ctx context.Context, dateWIB time.Time) ([]OrderSummary, error)
}
