// Package service — POS/walk-in orchestrator (§11).
//
// Thin: tidak punya storage sendiri. Alur:
//   1. Normalize + validate customer phone.
//   2. Resolve/create customer via authapi.CustomerService.
//   3. Call orderapi.CreatePOSOrder (atomic: order + history di status dibayar).
//   4. Trigger WA konfirmasi (KindPOSOrderCreated) — non-blocking.
//   5. Trigger auto-generate invoice — non-blocking.
//   6. Kembalikan result ke handler untuk cetak struk & display.
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
	"github.com/rajaku-printing/backend/internal/invoice/invoiceapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
)

type Config struct {
	// BaseURL untuk build tracking URL yg dikembalikan ke kasir.
	BaseURL string
}

type Service struct {
	orderCmd   orderapi.OrderCommandService
	customers  authapi.CustomerService
	notifier   notificationapi.Enqueuer
	invoiceGen invoiceapi.Generator
	cfg        Config
}

func New(orderCmd orderapi.OrderCommandService, customers authapi.CustomerService, cfg Config) *Service {
	return &Service{orderCmd: orderCmd, customers: customers, cfg: cfg}
}

func (s *Service) SetNotifier(n notificationapi.Enqueuer)          { s.notifier = n }
func (s *Service) SetInvoiceGenerator(g invoiceapi.Generator)      { s.invoiceGen = g }

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
		DesignSource:           in.DesignSource,
		DesignApprovalMode:     in.DesignApprovalMode,
		DesignBrief:            in.DesignBrief,
		Notes:                  in.Notes,
	})
	if err != nil {
		// Bubble up known orderapi errors; wrap sisanya.
		if errors.Is(err, orderapi.ErrShippingFieldsRequired) ||
			errors.Is(err, orderapi.ErrInvalidShippingCost) ||
			errors.Is(err, orderapi.ErrResiCollisionGaveUp) {
			return nil, err
		}
		// Deteksi metode_bayar invalid via string check (order.Service
		// return plain fmt.Errorf untuk validasi tipe primitive).
		msg := err.Error()
		if strings.Contains(msg, "metode_bayar POS") {
			return nil, posapi.ErrInvalidMetodeBayar
		}
		return nil, fmt.Errorf("%w: %v", posapi.ErrOrderCreate, err)
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
	}

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

// ---- helpers ----

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
