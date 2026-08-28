// Package service — POS/walk-in orchestrator (§11).
//
// Thin: tidak punya storage sendiri. Alur:
//  1. Normalize + validate customer phone.
//  2. Resolve/create customer via authapi.CustomerService.
//  3. Call orderapi.CreatePOSOrder (atomic: order + history di status dibayar).
//  4. Trigger WA konfirmasi (KindPOSOrderCreated) — non-blocking.
//  5. Trigger auto-generate invoice — non-blocking.
//  6. Kembalikan result ke handler untuk cetak struk & display.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
	"github.com/rajaku-printing/backend/internal/settings/settingsapi"
)

// defaultReceiptWidthMM — fallback lebar kertas struk (mm) kalau setting
// pos.receipt_width_mm tidak tersedia/rusak. Sesuai default seed migration
// 000022 — roll thermal paling umum untuk kasir kecil.
const defaultReceiptWidthMM = settingsapi.DefaultPOSReceiptWidthMM

// customerSearchLimit — jumlah hasil maksimum GET /admin/pos/customers/search
// (§11). Cukup untuk dropdown kasir tanpa perlu pagination.
const customerSearchLimit = 10

type Config struct {
	// BaseURL untuk build tracking URL yg dikembalikan ke kasir.
	BaseURL string
}

type Service struct {
	orderCmd   orderapi.OrderCommandService
	customers  authapi.CustomerService
	notifier   notificationapi.Enqueuer
	invoiceGen invoiceapi.Generator
	settings   settingsapi.Reader
	cfg        Config
}

func New(orderCmd orderapi.OrderCommandService, customers authapi.CustomerService, cfg Config) *Service {
	return &Service{orderCmd: orderCmd, customers: customers, cfg: cfg}
}

func (s *Service) SetNotifier(n notificationapi.Enqueuer)     { s.notifier = n }
func (s *Service) SetInvoiceGenerator(g invoiceapi.Generator) { s.invoiceGen = g }
func (s *Service) SetSettingsReader(r settingsapi.Reader)     { s.settings = r }

// CreateOrder — main entrypoint dari handler kasir.
func (s *Service) CreateOrder(ctx context.Context, in CreateOrderInput) (*CreateOrderResult, error) {
	// 1. Normalize phone.
	if strings.TrimSpace(in.CustomerName) == "" {
		return nil, fmt.Errorf("nama pelanggan wajib diisi")
	}
	phoneNorm, err := phone.Normalize(in.CustomerPhone)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", posapi.ErrInvalidPhone, err)
	}

	// 2. Resolve/create customer (matching key nomor WA — §11).
	identity, err := s.customers.ResolveOrCreateGuest(ctx, phoneNorm, in.CustomerName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", posapi.ErrCustomerResolve, err)
	}

	// 3. Create order via orderapi (atomic; skip menunggu_pembayaran).
	sum, err := s.orderCmd.CreatePOSOrder(ctx, orderapi.POSCreateOrderInput{
		CustomerID:             identity.UserID,
		KasirID:                in.KasirID,
		ProductID:              in.ProductID,
		MaterialID:             in.MaterialID,
		WidthCm:                in.WidthCm,
		HeightCm:               in.HeightCm,
		Quantity:               in.Quantity,
		MetodeAmbil:            in.MetodeAmbil,
		ShippingAddress:        in.ShippingAddress,
		ShippingRecipientName:  in.ShippingRecipientName,
		ShippingRecipientPhone: in.ShippingRecipientPhone,
		ShippingCost:           in.ShippingCost,
		MetodeBayar:            in.MetodeBayar,
		DiscountID:             in.DiscountID,
		ManualDiscountAmount:   in.ManualDiscountAmount,
		DiscountNote:           in.DiscountNote,
		DesignSource:           in.DesignSource,
		DesignApprovalMode:     in.DesignApprovalMode,
		DesignBrief:            in.DesignBrief,
		Notes:                  in.Notes,
	})
	if err != nil {
		// Bubble up known orderapi/discountapi errors; wrap sisanya.
		if errors.Is(err, orderapi.ErrShippingFieldsRequired) ||
			errors.Is(err, orderapi.ErrInvalidShippingCost) ||
			errors.Is(err, orderapi.ErrResiCollisionGaveUp) ||
			errors.Is(err, orderapi.ErrDiscountUnavailable) ||
			errors.Is(err, discountapi.ErrDiscountNotFound) ||
			errors.Is(err, discountapi.ErrDiscountInactive) ||
			errors.Is(err, discountapi.ErrDiscountNotStarted) ||
			errors.Is(err, discountapi.ErrDiscountExpired) ||
			errors.Is(err, discountapi.ErrDiscountChannelMismatch) ||
			errors.Is(err, discountapi.ErrDiscountMinSubtotal) ||
			errors.Is(err, discountapi.ErrDiscountQuotaExhausted) ||
			errors.Is(err, discountapi.ErrManualDiscountNoteRequired) ||
			errors.Is(err, discountapi.ErrManualDiscountInvalidAmount) ||
			errors.Is(err, discountapi.ErrDiscountAmbiguousInput) {
			return nil, err
		}
		// Deteksi metode_bayar invalid via string check (order.Service
		// return plain fmt.Errorf untuk validasi tipe primitive).
		msg := err.Error()
		if strings.Contains(msg, "metode_bayar POS") {
			return nil, posapi.ErrInvalidMetodeBayar
		}
		// %w dua kali (Go 1.20+, lihat go.mod): errors.Is(hasil,
		// posapi.ErrOrderCreate) TETAP selalu true (kode di atas ini sudah
		// menangkap sentinel yang punya penanganan HTTP khusus; baris ini
		// adalah fallback generic), TAPI err aslinya (mis. sentinel diskon
		// dari orderapi/discountapi yang belum dikenal handler versi ini,
		// atau error internal lain) juga tetap bisa dicek errors.Is sampai
		// ke handler — dulu dibungkus %v di sini yang memutus rantai itu,
		// membuat sentinel apa pun jatuh ke case ErrOrderCreate (500 generic)
		// alih-alih case spesifiknya masing-masing.
		return nil, fmt.Errorf("%w: %w", posapi.ErrOrderCreate, err)
	}

	result := &CreateOrderResult{
		OrderID:     sum.ID,
		Resi:        sum.Resi,
		CustomerID:  sum.CustomerID,
		Total:       sum.Total,
		MetodeBayar: sum.MetodeBayar,
		MetodeAmbil: sum.MetodeAmbil,
		CreatedAt:   sum.CreatedAt,
		TrackingURL: fmt.Sprintf("%s/lacak/%s", s.cfg.BaseURL, sum.Resi),
		// Nama SENGAJA dari input kasir, bukan identity.Name. identity berasal
		// dari ResolveOrCreateGuest yang, kalau nomor WA sudah terdaftar,
		// mengembalikan nama LAMA di DB dan mengabaikan nama yang baru
		// diketik kasir. Nomor WA adalah matching key (§11) — satu nomor
		// rumah/toko wajar dipakai beberapa orang, jadi nama bisa berbeda
		// antar kunjungan. Mencetak nama lama berarti struk yang diserahkan
		// ke pelanggan menyebut orang lain, sekaligus membocorkan siapa yang
		// terdaftar di nomor itu. Telepon tetap dari identity karena itu
		// versi ternormalisasi 62xxx (§13).
		CustomerName:   strings.TrimSpace(in.CustomerName),
		CustomerPhone:  identity.Phone,
		ProductName:    sum.ProductName,
		MaterialName:   sum.MaterialName,
		WidthCm:        sum.WidthCm,
		HeightCm:       sum.HeightCm,
		Quantity:       sum.Quantity,
		UnitPrice:      sum.UnitPrice,
		Subtotal:       sum.Subtotal,
		DiscountAmount: sum.DiscountAmount,
		DiscountLabel:  sum.DiscountLabel,
		ShippingCost:   sum.ShippingCost,
	}
	// Lebar kertas struk — order sudah tersimpan di atas, kegagalan baca
	// setting ini cuma menurunkan kualitas struk (fallback default), jangan
	// gagalkan pembuatan order.
	result.ReceiptWidthMM = s.resolveReceiptWidthMM(ctx)

	// 4. Trigger WA konfirmasi ke customer — best-effort, non-blocking.
	s.enqueuePOSNotif(ctx, sum.ID)

	// 5. Auto-generate invoice — best-effort. Kalau sukses, isi URL ke result.
	if info := s.autoGenerateInvoice(ctx, sum.ID, in.KasirID); info != nil {
		result.InvoiceURL = info.DownloadURL
		result.InvoiceNumber = info.InvoiceNumber
	}

	return result, nil
}

// DailyReconciliation — laporan orders POS per tanggal (WIB), pecah per
// metode_bayar & per kasir (§11).
func (s *Service) DailyReconciliation(ctx context.Context, dateWIB time.Time) (*ReconciliationReport, error) {
	rows, err := s.orderCmd.ListPOSOrdersByDate(ctx, dateWIB)
	if err != nil {
		return nil, fmt.Errorf("list POS orders: %w", err)
	}
	report := &ReconciliationReport{
		Date:          dateWIB,
		TotalOrders:   len(rows),
		ByMetodeBayar: map[string]MethodStats{},
	}
	kasirIdx := map[uuid.UUID]int{}
	for _, r := range rows {
		report.TotalRevenue += r.Total

		// By metode_bayar
		mb := r.MetodeBayar
		if mb == "" {
			mb = "unknown"
		}
		stat := report.ByMetodeBayar[mb]
		stat.Count++
		stat.Revenue += r.Total
		report.ByMetodeBayar[mb] = stat

		// By kasir
		if r.CreatedBy != nil {
			idx, ok := kasirIdx[*r.CreatedBy]
			if !ok {
				report.ByKasir = append(report.ByKasir, KasirStats{KasirID: *r.CreatedBy})
				idx = len(report.ByKasir) - 1
				kasirIdx[*r.CreatedBy] = idx
			}
			report.ByKasir[idx].Count++
			report.ByKasir[idx].Revenue += r.Total
		}
	}
	return report, nil
}

// ReceiptConfig returns the active receipt width (§12) plus the full list of
// widths the system supports, for staff-facing UI (e.g. reprint struk dari
// halaman detail order) yang tidak boleh di-gate lewat permission
// settings.manage (lihat pos/handler/routes.go untuk alasan lengkap).
func (s *Service) ReceiptConfig(ctx context.Context) ReceiptConfig {
	// Copy, bukan slice aslinya: settingsapi.POSReceiptWidthsMM adalah var
	// global yang dipromosikan sebagai sumber kebenaran tunggal. Menyerahkan
	// header slice-nya ke luar bikin daftar itu bisa dimutasi dari jarak jauh.
	allowed := make([]int, len(settingsapi.POSReceiptWidthsMM))
	copy(allowed, settingsapi.POSReceiptWidthsMM)
	return ReceiptConfig{
		WidthMM:         s.resolveReceiptWidthMM(ctx),
		AllowedWidthsMM: allowed,
	}
}

// SearchCustomers — cari pelanggan existing by nama/WA (§11), dipakai layar
// kasir supaya tidak input ulang data pelanggan yang sudah pernah order.
func (s *Service) SearchCustomers(ctx context.Context, q string) ([]CustomerSearchResult, error) {
	identities, err := s.customers.SearchCustomers(ctx, q, customerSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search customers: %w", err)
	}
	out := make([]CustomerSearchResult, 0, len(identities))
	for _, id := range identities {
		out = append(out, CustomerSearchResult{ID: id.UserID, Name: id.Name, Phone: id.Phone, MembershipStatus: id.MembershipStatus})
	}
	return out, nil
}

// ---- helpers ----

// resolveReceiptWidthMM membaca setting pos.receipt_width_mm; fallback ke
// defaultReceiptWidthMM kalau reader belum di-wire, GetInt error (row hilang/
// rusak), atau nilainya bukan 58/80 (mis. row diedit manual langsung di DB,
// lolos dari validasi enum settings/service). Fallback SELALU disertai log
// warn — jangan menelan kegagalan konfigurasi diam-diam (§22).
func (s *Service) resolveReceiptWidthMM(ctx context.Context) int {
	if s.settings == nil {
		log.Ctx(ctx).Warn().
			Int("fallback_mm", defaultReceiptWidthMM).
			Msg("pos: settings reader belum di-wire, pakai fallback lebar struk")
		return defaultReceiptWidthMM
	}
	width, err := s.settings.GetInt(ctx, settingsapi.KeyPOSReceiptWidthMM)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).
			Int("fallback_mm", defaultReceiptWidthMM).
			Msg("pos: gagal baca setting pos.receipt_width_mm, pakai fallback")
		return defaultReceiptWidthMM
	}
	if !settingsapi.IsValidPOSReceiptWidthMM(width) {
		log.Ctx(ctx).Warn().
			Int("value", width).
			Int("fallback_mm", defaultReceiptWidthMM).
			Msg("pos: nilai pos.receipt_width_mm di luar daftar lebar yang didukung, pakai fallback")
		return defaultReceiptWidthMM
	}
	return width
}

func (s *Service) enqueuePOSNotif(ctx context.Context, orderID uuid.UUID) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.EnqueueOrderEvent(ctx, notificationapi.KindPOSOrderCreated, orderID, nil); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Msg("enqueue pos_order_created WA failed — kasir sudah cetak struk, WA konfirmasi bisa dikirim manual")
	}
}

// autoGenerateInvoice — return info hanya kalau sukses (untuk isi struk URL).
// Kalau nil, invoice bisa diregenerate manual dari admin panel nanti.
func (s *Service) autoGenerateInvoice(ctx context.Context, orderID uuid.UUID, kasirID uuid.UUID) *invoiceapi.Info {
	if s.invoiceGen == nil {
		return nil
	}
	info, err := s.invoiceGen.GenerateForOrder(ctx, orderID, &kasirID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Msg("auto-generate POS invoice failed — order sudah tersimpan, invoice bisa regenerate manual")
		return nil
	}
	return info
}
