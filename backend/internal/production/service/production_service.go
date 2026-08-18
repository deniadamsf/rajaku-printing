package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/production/productionapi"
)

// Service — orchestrator produksi. Tidak punya storage sendiri; semua state
// hidup di orders table via orderCmd. Notifier best-effort per §13.
type Service struct {
	orderCmd orderapi.OrderCommandService
	notifier notificationapi.Enqueuer
}

func New(orderCmd orderapi.OrderCommandService) *Service {
	return &Service{orderCmd: orderCmd}
}

// SetNotifier — dipanggil router.go setelah notification.Service dibuat.
func (s *Service) SetNotifier(n notificationapi.Enqueuer) { s.notifier = n }

// ---- Advance operations ----
//
// Semua method resolve resi → orderID lewat orderCmd, lalu delegasikan ke
// order state-machine command. Error dari orderapi di-map ke productionapi
// sentinel oleh mapOrderErr — handler tidak perlu tahu error orderapi.

// resolveOrderID — helper resolve resi + return summary (untuk cek metode_ambil).
func (s *Service) resolveOrderID(ctx context.Context, resi string) (uuid.UUID, *orderapi.OrderSummary, error) {
	sum, err := s.orderCmd.FindSummaryByResi(ctx, resi)
	if err != nil {
		if errors.Is(err, orderapi.ErrOrderNotFound) {
			return uuid.Nil, nil, productionapi.ErrOrderNotFound
		}
		return uuid.Nil, nil, fmt.Errorf("lookup order: %w", err)
	}
	return sum.ID, sum, nil
}

// StartCetak — advance desain_diverifikasi → proses_cetak. Tidak trigger WA
// (internal step; hindari spam).
func (s *Service) StartCetak(ctx context.Context, in AdvanceInput) error {
	orderID, _, err := s.resolveOrderID(ctx, in.Resi)
	if err != nil {
		return err
	}
	if err := s.orderCmd.MarkProsesCetak(ctx, orderID, &in.StaffID, in.Note); err != nil {
		return mapOrderErr(err)
	}
	return nil
}

// StartQC — advance proses_cetak → qc. No WA notif (internal step).
func (s *Service) StartQC(ctx context.Context, in AdvanceInput) error {
	orderID, _, err := s.resolveOrderID(ctx, in.Resi)
	if err != nil {
		return err
	}
	if err := s.orderCmd.MarkQC(ctx, orderID, &in.StaffID, in.Note); err != nil {
		return mapOrderErr(err)
	}
	return nil
}

// MarkSiap — lulus QC. Trigger WA:
//   - metode_ambil=kirim  → KindReadyShip
//   - metode_ambil=pickup → KindReadyPickup
// (order service auto-branch status internally).
func (s *Service) MarkSiap(ctx context.Context, in AdvanceInput) error {
	orderID, sum, err := s.resolveOrderID(ctx, in.Resi)
	if err != nil {
		return err
	}
	if err := s.orderCmd.MarkSiapKirimAtauAmbil(ctx, orderID, &in.StaffID, in.Note); err != nil {
		return mapOrderErr(err)
	}
	// Pilih kind berdasarkan metode_ambil ORDER (bukan target state, karena
	// target dihitung di order.Service).
	kind := notificationapi.KindReadyShip
	if sum.MetodeAmbil == "pickup" {
		kind = notificationapi.KindReadyPickup
	}
	s.enqueueNotif(ctx, kind, orderID, nil)
	return nil
}

// MarkShipped — advance siap_kirim → dikirim, record courier+tracking, kirim WA.
func (s *Service) MarkShipped(ctx context.Context, in MarkShippedInput) error {
	orderID, _, err := s.resolveOrderID(ctx, in.Resi)
	if err != nil {
		return err
	}
	if err := s.orderCmd.MarkDikirim(ctx, orderID, &in.StaffID, in.Courier, in.TrackingNumber, in.Note); err != nil {
		return mapOrderErr(err)
	}
	s.enqueueNotif(ctx, notificationapi.KindShipped, orderID, map[string]any{
		"courier":  in.Courier,
		"tracking": in.TrackingNumber,
	})
	return nil
}

// MarkSelesai — advance dikirim/siap_ambil → selesai. Tidak trigger WA
// (customer sudah tahu — dikirim dgn resi ekspedisi, atau ambil langsung).
func (s *Service) MarkSelesai(ctx context.Context, in AdvanceInput) error {
	orderID, _, err := s.resolveOrderID(ctx, in.Resi)
	if err != nil {
		return err
	}
	if err := s.orderCmd.MarkSelesai(ctx, orderID, &in.StaffID, in.Note); err != nil {
		return mapOrderErr(err)
	}
	return nil
}

// ---- helpers ----

func mapOrderErr(err error) error {
	switch {
	case errors.Is(err, orderapi.ErrOrderNotFound):
		return productionapi.ErrOrderNotFound
	case errors.Is(err, orderapi.ErrOrderStateChanged):
		return productionapi.ErrOrderStateChanged
	case errors.Is(err, orderapi.ErrDikirimOnPickup):
		return productionapi.ErrDikirimOnPickup
	case errors.Is(err, orderapi.ErrShippingTrackingRequired):
		return productionapi.ErrShippingTrackingRequired
	default:
		// State-machine reject (bukan sentinel spesifik) → invalid transition.
		return productionapi.ErrInvalidTransition
	}
}

func (s *Service) enqueueNotif(ctx context.Context, kind notificationapi.Kind, orderID uuid.UUID, extras map[string]any) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.EnqueueOrderEvent(ctx, kind, orderID, extras); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", orderID.String()).
			Str("kind", string(kind)).
			Msg("enqueue production notification failed — WA tidak terkirim, cek manual")
	}
}
