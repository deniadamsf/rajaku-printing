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
	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
	"github.com/rajaku-printing/backend/internal/order/model"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/repository"
	"github.com/rajaku-printing/backend/internal/order/resi"
	"github.com/rajaku-printing/backend/internal/order/state"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

const resiRetryAttempts = 8

// OrderStore is the storage contract the service depends on — narrowed to just
// what CreateOnlineOrder + tracking + admin need, so tests can supply a fake
// without pulling GORM. *repository.OrderRepository satisfies this.
type OrderStore interface {
	CreateWithHistory(ctx context.Context, order *model.Order, items []model.OrderItem, initialHistory *model.OrderStateHistory) error
	FindByResi(ctx context.Context, resi string) (*model.Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	FindHistoryByOrderID(ctx context.Context, orderID uuid.UUID) ([]model.OrderStateHistory, error)
	ListForAdmin(ctx context.Context, f repository.AdminListFilter) (*repository.AdminListResult, error)
	ListByCustomer(ctx context.Context, f repository.CustomerListFilter) (*repository.AdminListResult, error)
	ListPOSByDateRange(ctx context.Context, start, end time.Time) ([]model.Order, error)
	SetShippingCostAndAdvance(ctx context.Context, p repository.SetShippingCostParams) error
	AdvanceStatus(ctx context.Context, p repository.AdvanceStatusParams) error

	// --- Order recap (§28.5) ---
	RecapSummary(ctx context.Context, f repository.RecapFilter) (*repository.RecapSummary, error)
	RecapList(ctx context.Context, f repository.RecapFilter) ([]repository.RecapRow, error)
	// RecapListBatch — dipakai streaming CSV export (temuan review #4b),
	// SELALU offset/limit-bounded terlepas dari f.NoLimit/f.Page/f.PageSize.
	RecapListBatch(ctx context.Context, f repository.RecapFilter, offset, limit int) ([]repository.RecapRow, error)
	// RecapDistinctKasir/RecapDistinctDiscounts — dipakai
	// GET /admin/order-recap/filters (dropdown kasir/diskon, bukan kotak
	// teks UUID).
	RecapDistinctKasir(ctx context.Context, from, to time.Time) ([]repository.RecapKasirOption, error)
	RecapDistinctDiscounts(ctx context.Context, from, to time.Time) ([]repository.RecapDiscountSnapshotRow, error)

	// --- Super admin order tools (§ super admin order tools) ---
	UpdateFields(ctx context.Context, p repository.UpdateFieldsParams) error
	// UpdateItems — koreksi baris item pesanan oleh super admin (§32.9).
	UpdateItems(ctx context.Context, p repository.UpdateItemsParams) error
	OverrideStatus(ctx context.Context, p repository.OverrideStatusParams) (string, error)
	SoftDelete(ctx context.Context, p repository.SoftDeleteParams) error

	// --- Customer admin (orderapi.CustomerOrderReader, modul auth) ---
	CustomerOrderStats(ctx context.Context, customerID uuid.UUID) (*repository.CustomerOrderStatsRow, error)
	RecentOrdersByCustomer(ctx context.Context, customerID uuid.UUID, limit int) ([]repository.CustomerOrderBriefRow, error)
}

// AuditStore is the narrow read contract for admin_audit_log — kept separate
// from OrderStore because it isn't order-table-specific (entity_type may grow
// beyond "order" later). Wired via SetAuditStore (same pattern as
// SetNotifier) so New()'s existing call sites/tests don't need updating.
type AuditStore interface {
	ListByEntity(ctx context.Context, f repository.AdminAuditListFilter) ([]model.AdminAuditLog, error)
}

// Compile-time assertion — concrete audit repo satisfies the interface.
var _ AuditStore = (*repository.AdminAuditLogRepository)(nil)

// Compile-time assertion — concrete repo satisfies the interface.
var _ OrderStore = (*repository.OrderRepository)(nil)

// Compile-time assertion — Service satisfies the cross-module command contract
// so it can be injected into payment/notification modules via orderapi.
var _ orderapi.OrderCommandService = (*Service)(nil)

// Service wires order storage dgn dependencies eksternal (catalog untuk
// quote, auth customer service untuk guest resolve). Semua via interface,
// tidak import package internal modul lain (spec section 22).
type Service struct {
	orders    OrderStore
	catalog   catalogapi.CatalogService
	customers authapi.CustomerService
	// notifier — opsional; kalau nil, transisi status tidak trigger WA.
	// Best-effort per §13: enqueue error di-log, tidak rollback transaksi.
	notifier notificationapi.Enqueuer
	// audit — opsional; kalau nil, ListAuditLog error (§ super admin order
	// tools). Wired via SetAuditStore after router builds the repository.
	audit AuditStore
	// notifCanceller — opsional; kalau nil, SoftDeleteOrder tidak membatalkan
	// job WA yang masih antre untuk order yang dihapus (§ super admin order
	// tools review finding #5). Wired via SetNotificationCanceller, same
	// pattern as SetNotifier.
	notifCanceller notificationapi.JobCanceller
	// discount — opsional; kalau nil DAN sebuah order membawa discount_id
	// atau manual discount amount, CreatePOSOrder menolak eksplisit dengan
	// orderapi.ErrDiscountUnavailable (§22 no-silent-stub — TIDAK diam-diam
	// mengabaikan diskon yang diminta). Wired via SetDiscountResolver, same
	// setter-injection pattern as SetNotifier (discount.Service dibuat
	// setelah order.Service di composition root).
	discount discountapi.Resolver
}

func New(orders OrderStore, catalog catalogapi.CatalogService, customers authapi.CustomerService) *Service {
	return &Service{orders: orders, catalog: catalog, customers: customers}
}

// SetNotifier wires an optional Enqueuer. Called by router.go after
// notification.Service dibuat — dilakukan sebagai setter (bukan constructor
// arg) supaya circular wiring (order.Service dibuat sebelum notification.Service
// karena notif butuh orderCmd) tetap kompilasi tanpa refactor besar.
//
// Aman dipanggil sekali saat startup; setelah router siap, tidak boleh diubah
// runtime (no concurrent write protection here).
func (s *Service) SetNotifier(n notificationapi.Enqueuer) { s.notifier = n }

// SetAuditStore wires the admin_audit_log reader. Same rationale/pattern as
// SetNotifier — see admin_override.go for consumers (ListAuditLog).
func (s *Service) SetAuditStore(a AuditStore) { s.audit = a }

// SetNotificationCanceller wires an optional JobCanceller. Same
// rationale/pattern as SetNotifier — see admin_override.go's
// cancelPendingNotifications (SoftDeleteOrder).
func (s *Service) SetNotificationCanceller(c notificationapi.JobCanceller) { s.notifCanceller = c }

// SetDiscountResolver wires an optional discountapi.Resolver (§28). Same
// rationale/pattern as SetNotifier — see CreatePOSOrder for the consumer.
func (s *Service) SetDiscountResolver(r discountapi.Resolver) { s.discount = r }

// CreateOnlineOrder creates a new order coming from the web (channel=online).
// Kalau CustomerID nil, service akan resolve/create guest via CustomerService
// pakai GuestPhone + GuestName.
//
// Semua validasi bisnis di sini — handler cukup validate shape (binding tag).
func (s *Service) CreateOnlineOrder(ctx context.Context, in CreateOnlineOrderInput) (*model.Order, error) {
	// 1. Resolve customer identity
	customerID, err := s.resolveCustomer(ctx, in)
	if err != nil {
		return nil, err
	}

	// 2. Validate item count (§32.4) + design/pickup fields
	if err := validateItemCount(len(in.Items)); err != nil {
		return nil, err
	}
	if err := s.validateOnlineInput(in); err != nil {
		return nil, err
	}

	// 3. Normalize shipping phone (if kirim)
	shippingPhoneNorm := ""
	if in.MetodeAmbil == model.MetodeAmbilKirim {
		shippingPhoneNorm, err = phone.Normalize(in.ShippingRecipientPhone)
		if err != nil {
			return nil, fmt.Errorf("shipping recipient phone: %w", err)
		}
	}

	// 4. Quote setiap item via catalog (single source of truth — jangan
	// trust harga dari client) + build order_items (§32).
	items, subtotal, err := s.quoteOnlineItems(ctx, in.Items)
	if err != nil {
		return nil, err
	}
	total := subtotal // shipping_cost belum ada di create — diisi admin nanti

	// 5. Build order struct
	order := &model.Order{
		CustomerID:   customerID,
		Channel:      model.ChannelOnline,
		Status:       state.OrderMasuk,
		Subtotal:     subtotal,
		MetodeAmbil:  in.MetodeAmbil,
		DesignSource: deriveDesignSource(items),
		Total:        total,
	}
	if in.MetodeAmbil == model.MetodeAmbilKirim {
		order.ShippingAddress = strPtr(in.ShippingAddress)
		order.ShippingRecipientName = strPtr(in.ShippingRecipientName)
		order.ShippingRecipientPhone = strPtr(shippingPhoneNorm)
	}
	if in.Notes != "" {
		order.Notes = strPtr(in.Notes)
	}

	// 6. Insert with retry-on-collision (resi unique)
	if err := s.createWithResiRetry(ctx, order, items, state.OrderMasuk, nil, nil); err != nil {
		return nil, err
	}
	return order, nil
}

// quoteOnlineItems quotes each requested item via catalog (authoritative
// pricing — jangan trust harga dari client) and builds the model.OrderItem
// rows + aggregate subtotal for CreateOnlineOrder. LineNo starts at 1 in
// request order.
func (s *Service) quoteOnlineItems(ctx context.Context, in []CreateOnlineOrderItemInput) ([]model.OrderItem, int64, error) {
	items := make([]model.OrderItem, 0, len(in))
	var subtotal int64
	for i, it := range in {
		if it.DesignSource != model.DesignSourceUpload && it.DesignSource != model.DesignSourceRequest {
			return nil, 0, fmt.Errorf("item %d: design_source %q: %w", i+1, it.DesignSource, orderapi.ErrInvalidDesignSource)
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		quote, err := s.catalog.Quote(ctx, catalogapi.QuoteRequest{
			ProductID:  it.ProductID,
			MaterialID: it.MaterialID,
			WidthCm:    it.WidthCm,
			HeightCm:   it.HeightCm,
		})
		if err != nil {
			// §22 — dibungkus dengan nomor baris: dengan sampai 20 item per
			// order, "produk tidak ditemukan" polos tidak memberi tahu baris
			// mana yang bermasalah. %w tetap dipertahankan supaya
			// errors.Is(err, catalogapi.Err...) di handler.mapErr tetap
			// bekerja lewat rantai wrapping.
			return nil, 0, fmt.Errorf("item %d: quote: %w", i+1, err)
		}
		lineSubtotal := quote.TotalPrice * int64(qty)
		subtotal += lineSubtotal
		item := model.OrderItem{
			LineNo:               i + 1,
			ProductID:            &quote.ProductID,
			ProductNameSnapshot:  quote.ProductName,
			MaterialID:           &quote.MaterialID,
			MaterialNameSnapshot: quote.MaterialName,
			PricingTypeSnapshot:  string(quote.PricingType),
			WidthCm:              it.WidthCm,
			HeightCm:             it.HeightCm,
			Quantity:             qty,
			UnitPrice:            quote.TotalPrice,
			Subtotal:             lineSubtotal,
			DesignSource:         it.DesignSource,
			ChargeableM2:         quote.ChargeableM2 * float64(qty),
		}
		if it.DesignBrief != "" {
			item.DesignBrief = strPtr(it.DesignBrief)
		}
		items = append(items, item)
	}
	return items, subtotal, nil
}

// GetByResiForOwner returns full order details for the requesting identity.
// Rules:
//   - Staff (user_type=staff): boleh lihat semua order.
//   - Customer: hanya boleh lihat order milik customer_id-nya sendiri.
//
// Kalau resi tidak ada → ErrOrderNotFound. Kalau caller bukan owner & bukan staff
// → ErrNotOwner (handler maps ke 403).
func (s *Service) GetByResiForOwner(ctx context.Context, resiStr string, caller *authapi.Identity) (*model.Order, error) {
	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order by resi: %w", err)
	}
	if caller == nil {
		return nil, orderapi.ErrNotOwner
	}
	if caller.UserType == authapi.UserTypeStaff {
		return o, nil
	}
	if o.CustomerID != caller.UserID {
		return nil, orderapi.ErrNotOwner
	}
	return o, nil
}

// GetByResiPublic returns censored data for anonymous tracking.
func (s *Service) GetByResiPublic(ctx context.Context, resiStr string) (*PublicTrackingResult, error) {
	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("get public tracking: %w", err)
	}
	history, err := s.orders.FindHistoryByOrderID(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	rows := make([]PublicTrackingResultHistoryRow, 0, len(history))
	for _, h := range history {
		rows = append(rows, PublicTrackingResultHistoryRow{
			Status:    string(h.ToStatus),
			ChangedAt: h.ChangedAt.UTC().Format(time.RFC3339),
		})
	}
	out := &PublicTrackingResult{
		Resi:         o.Resi,
		Status:       string(o.Status),
		Channel:      string(o.Channel),
		MetodeAmbil:  string(o.MetodeAmbil),
		DesignSource: string(o.DesignSource),
		Items:        publicTrackingItems(o.Items),
		CreatedAt:    o.CreatedAt.UTC().Format(time.RFC3339),
		History:      rows,
	}
	if o.MetodeAmbil == model.MetodeAmbilKirim {
		if o.ShippingRecipientName != nil {
			out.ShippingRecipient = maskName(*o.ShippingRecipientName)
		}
		if o.ShippingRecipientPhone != nil {
			out.ShippingPhoneMasked = maskPhone(*o.ShippingRecipientPhone)
		}
		if o.ShippingAddress != nil {
			out.ShippingAddressMasked = maskAddress(*o.ShippingAddress)
		}
	}
	return out, nil
}

// AdminGetDetail returns order + customer info (full, un-censored) + history
// rows. Caller-side permission (order.view) sudah dicek middleware.
//
// History rows include from/to/note/changed_by (UUID). Frontend bisa deref
// changed_by ke nama staff kalau perlu — untuk MVP kita return raw UUID.
func (s *Service) AdminGetDetail(ctx context.Context, resiStr string) (*model.Order, *AdminOrderDetail, error) {
	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, orderapi.ErrOrderNotFound
		}
		return nil, nil, fmt.Errorf("admin get detail: %w", err)
	}

	// Load customer identity (best-effort — jangan gagal detail kalau customer
	// hilang, tinggal skip).
	detail := &AdminOrderDetail{}
	if s.customers != nil {
		if ident, cerr := s.customers.FindByID(ctx, o.CustomerID); cerr == nil && ident != nil {
			ctype := ""
			// Guest jika email kosong — proxy sederhana (authapi.Identity tidak
			// expose customer_type). Bisa direfine nanti kalau butuh presisi.
			if ident.Email == "" {
				ctype = "guest"
			} else {
				ctype = "registered"
			}
			detail.CustomerInfo = &AdminOrderCustomer{
				ID:    ident.UserID.String(),
				Name:  ident.Name,
				Phone: ident.Phone,
				Email: ident.Email,
				Type:  ctype,
			}
		}
	}

	// History rows
	history, err := s.orders.FindHistoryByOrderID(ctx, o.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("admin get detail: load history: %w", err)
	}
	detail.History = make([]AdminHistoryRow, 0, len(history))
	for _, h := range history {
		row := AdminHistoryRow{
			ID:        h.ID.String(),
			ToStatus:  string(h.ToStatus),
			ChangedAt: h.ChangedAt.UTC().Format(time.RFC3339),
		}
		if h.FromStatus != nil {
			fs := string(*h.FromStatus)
			row.FromStatus = &fs
		}
		if h.ChangedBy != nil {
			cb := h.ChangedBy.String()
			row.ChangedBy = &cb
		}
		if h.Note != nil {
			row.Note = *h.Note
		}
		detail.History = append(detail.History, row)
	}
	return o, detail, nil
}

// ListForCustomer — dipakai halaman /akun (customer melihat pesanannya sendiri).
// Filter by caller.UserID. Kalau caller staff, tolak — endpoint ini eksklusif
// untuk customer melihat data sendiri (staff pakai /admin/orders).
type CustomerListInput struct {
	CustomerID uuid.UUID
	Status     string
	Page       int
	PageSize   int
}

func (s *Service) ListForCustomer(ctx context.Context, in CustomerListInput) (*AdminListPage, error) {
	res, err := s.orders.ListByCustomer(ctx, repository.CustomerListFilter{
		CustomerID: in.CustomerID,
		Status:     in.Status,
		Page:       in.Page,
		PageSize:   in.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list customer orders: %w", err)
	}
	return &AdminListPage{
		Items:    res.Items,
		Total:    res.Total,
		Page:     res.Page,
		PageSize: res.PageSize,
	}, nil
}

// ListForAdmin returns a paginated slice of orders matching the filter.
// Caller-side permission check (RequirePermission "order.view") happens in
// middleware — this method assumes staff-level access.
func (s *Service) ListForAdmin(ctx context.Context, in AdminListInput) (*AdminListPage, error) {
	res, err := s.orders.ListForAdmin(ctx, repository.AdminListFilter{
		Status:   in.Status,
		Channel:  in.Channel,
		Page:     in.Page,
		PageSize: in.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list admin orders: %w", err)
	}
	return &AdminListPage{
		Items:    res.Items,
		Total:    res.Total,
		Page:     res.Page,
		PageSize: res.PageSize,
	}, nil
}

// SetShippingCost fills ongkir + advances state to menunggu_pembayaran.
// Rules:
//   - Only valid when metode_ambil == kirim.
//   - Current status must be order_masuk (fresh order) or menunggu_ongkir
//     (already acknowledged by another staff). Anything else → ErrInvalidTransition.
//   - ShippingCost must be >= 0.
//
// From order_masuk: 2 history rows recorded (→ menunggu_ongkir → menunggu_pembayaran).
// From menunggu_ongkir: 1 history row (→ menunggu_pembayaran).
func (s *Service) SetShippingCost(ctx context.Context, in SetShippingCostInput) (*model.Order, error) {
	if in.ShippingCost < 0 {
		return nil, orderapi.ErrInvalidShippingCost
	}
	o, err := s.orders.FindByResi(ctx, in.Resi)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("lookup order: %w", err)
	}
	if o.MetodeAmbil != model.MetodeAmbilKirim {
		return nil, orderapi.ErrShippingCostOnPickup
	}
	var intermediate string
	switch o.Status {
	case state.OrderMasuk:
		intermediate = string(state.MenungguOngkir)
	case state.MenungguOngkir:
		intermediate = ""
	default:
		return nil, orderapi.ErrInvalidTransition
	}

	// §28.3 — ongkir tidak pernah didiskon; diskon hanya memotong subtotal.
	newTotal := o.Subtotal - o.DiscountAmount + in.ShippingCost
	params := repository.SetShippingCostParams{
		OrderID:      o.ID,
		ShippingCost: in.ShippingCost,
		NewTotal:     newTotal,
		FromStatus:   string(o.Status),
		NewStatus:    string(state.MenungguPembayaran),
		Intermediate: intermediate,
		ChangedBy:    &in.StaffID,
	}
	if in.Note != "" {
		note := in.Note
		params.Note = &note
	}
	if err := s.orders.SetShippingCostAndAdvance(ctx, params); err != nil {
		if errors.Is(err, repository.ErrStaleState) {
			return nil, orderapi.ErrOrderStateChanged
		}
		return nil, fmt.Errorf("set shipping cost: %w", err)
	}

	// Re-read so caller returns the fresh state (status, total, shipping_cost).
	updated, err := s.orders.FindByResi(ctx, in.Resi)
	if err != nil {
		return nil, fmt.Errorf("reload order after ongkir: %w", err)
	}

	s.enqueueOngkirReady(ctx, updated, in.ShippingCost)
	return updated, nil
}

// ConfirmPickupTotal advances a pickup order from order_masuk to
// menunggu_pembayaran (no ongkir to fill; admin just acknowledges the total).
// Only valid when metode_ambil == pickup && current status == order_masuk.
//
// Discount (§28.3): this does NOT recompute Total — for a pickup order,
// Total was already set correctly (Subtotal - DiscountAmount + 0) at
// creation time by CreateOnlineOrder/CreatePOSOrder, and this method never
// touches Subtotal/ShippingCost/DiscountAmount, so there's nothing to
// recompute here. (CreateOnlineOrder itself never carries a discount — §28
// forbids public promo codes on the online order form — so DiscountAmount is
// always 0 for that path; a POS pickup order already resolves its discount
// in CreatePOSOrder before this method is ever reached.)
func (s *Service) ConfirmPickupTotal(ctx context.Context, resiStr string, staffID uuid.UUID, note string) (*model.Order, error) {
	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("lookup order: %w", err)
	}
	if o.MetodeAmbil != model.MetodeAmbilPickup {
		return nil, orderapi.ErrConfirmPickupOnKirim
	}
	if o.Status != state.OrderMasuk {
		return nil, orderapi.ErrInvalidTransition
	}

	params := repository.AdvanceStatusParams{
		OrderID:    o.ID,
		FromStatus: string(state.OrderMasuk),
		NewStatus:  string(state.MenungguPembayaran),
		ChangedBy:  &staffID,
	}
	if note != "" {
		n := note
		params.Note = &n
	}
	if err := s.orders.AdvanceStatus(ctx, params); err != nil {
		if errors.Is(err, repository.ErrStaleState) {
			return nil, orderapi.ErrOrderStateChanged
		}
		return nil, fmt.Errorf("confirm pickup total: %w", err)
	}
	updated, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		return nil, fmt.Errorf("reload order after confirm-pickup: %w", err)
	}

	// Pickup: no shipping cost — total = subtotal, sudah fix di order sebelumnya.
	s.enqueueOngkirReady(ctx, updated, 0)
	return updated, nil
}

// enqueueOngkirReady — best-effort trigger notifikasi setelah order masuk
// menunggu_pembayaran. Error di-log & di-swallow: WA gagal tidak boleh
// membatalkan konfirmasi ongkir yang sudah tersimpan di DB (§13 best-effort).
func (s *Service) enqueueOngkirReady(ctx context.Context, o *model.Order, shippingCost int64) {
	if s.notifier == nil {
		return
	}
	extras := map[string]any{}
	if shippingCost > 0 {
		extras["shipping_cost"] = shippingCost
	}
	if err := s.notifier.EnqueueOrderEvent(ctx, notificationapi.KindOngkirReady, o.ID, extras); err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("order_id", o.ID.String()).
			Str("resi", o.Resi).
			Str("kind", string(notificationapi.KindOngkirReady)).
			Msg("enqueue ongkir_ready notification failed — WA tidak terkirim, cek manual")
	}
}

// ---- OrderCommandService (consumed by payment/notification modules) ----

// FindSummaryByResi implements orderapi.OrderCommandService.
func (s *Service) FindSummaryByResi(ctx context.Context, resiStr string) (*orderapi.OrderSummary, error) {
	o, err := s.orders.FindByResi(ctx, resiStr)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("find summary by resi: %w", err)
	}
	return orderToSummary(o), nil
}

// FindSummaryByID implements orderapi.OrderCommandService.
func (s *Service) FindSummaryByID(ctx context.Context, id uuid.UUID) (*orderapi.OrderSummary, error) {
	o, err := s.orders.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("find summary by id: %w", err)
	}
	return orderToSummary(o), nil
}

// FindInvoiceViewByID implements orderapi.OrderCommandService.
func (s *Service) FindInvoiceViewByID(ctx context.Context, id uuid.UUID) (*orderapi.OrderInvoiceView, error) {
	o, err := s.orders.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, orderapi.ErrOrderNotFound
		}
		return nil, fmt.Errorf("find invoice view: %w", err)
	}
	return orderToInvoiceView(o), nil
}

func orderToInvoiceView(o *model.Order) *orderapi.OrderInvoiceView {
	v := &orderapi.OrderInvoiceView{
		ID:                 o.ID,
		Resi:               o.Resi,
		CustomerID:         o.CustomerID,
		CreatedAt:          o.CreatedAt,
		Channel:            string(o.Channel),
		Status:             string(o.Status),
		MetodeAmbil:        string(o.MetodeAmbil),
		Items:              itemsToView(o.Items),
		Subtotal:           o.Subtotal,
		DiscountAmount:     o.DiscountAmount,
		DiscountLabel:      discountLabel(o),
		Total:              o.Total,
		ShippingCost:       o.ShippingCost,
		ShippingAddress:    o.ShippingAddress,
		ShippingRecipient:  o.ShippingRecipientName,
		ShippingPhone:      o.ShippingRecipientPhone,
		ShippingCourier:    o.ShippingCourier,
		ShippingTrackingNo: o.ShippingTrackingNumber,
		Notes:              o.Notes,
	}
	if o.MetodeBayar != nil {
		v.MetodeBayar = string(*o.MetodeBayar)
	}
	return v
}

// itemsToView projects order_items (§32) into the orderapi-safe
// OrderItemView slice — shared by orderToSummary/orderToInvoiceView so the
// projection rule lives in exactly one place.
func itemsToView(items []model.OrderItem) []orderapi.OrderItemView {
	out := make([]orderapi.OrderItemView, 0, len(items))
	for i := range items {
		it := &items[i]
		view := orderapi.OrderItemView{
			ID:             it.ID,
			LineNo:         it.LineNo,
			ProductID:      it.ProductID,
			ProductName:    it.ProductNameSnapshot,
			MaterialName:   it.MaterialNameSnapshot,
			PricingType:    it.PricingTypeSnapshot,
			WidthCm:        it.WidthCm,
			HeightCm:       it.HeightCm,
			Quantity:       it.Quantity,
			UnitPrice:      it.UnitPrice,
			Subtotal:       it.Subtotal,
			DiscountAmount: it.DiscountAmount,
			DesignSource:   string(it.DesignSource),
		}
		if it.DesignBrief != nil {
			view.DesignBrief = *it.DesignBrief
		}
		if it.ItemNotes != nil {
			view.ItemNotes = *it.ItemNotes
		}
		out = append(out, view)
	}
	return out
}

// publicTrackingItems projects every order_item (§32) into the
// PublicTrackingResultItem shape safe for the no-login tracking endpoint
// (§5) — SEMUA baris, bukan cuma yang pertama (lihat PublicTrackingResult
// doc untuk kenapa proyeksi lama "+N lainnya" diganti dengan daftar lengkap).
// Order TIDAK PERNAH punya 0 item (§32.4); items kosong di sini berarti
// caller lupa preload, jadi mengembalikan slice kosong apa adanya (tidak
// panic) supaya gagalnya kelihatan di data, bukan crash.
func publicTrackingItems(items []model.OrderItem) []PublicTrackingResultItem {
	out := make([]PublicTrackingResultItem, 0, len(items))
	for i := range items {
		out = append(out, PublicTrackingResultItem{
			ID:           items[i].ID.String(),
			ProductName:  items[i].ProductNameSnapshot,
			MaterialName: items[i].MaterialNameSnapshot,
			WidthCm:      items[i].WidthCm,
			HeightCm:     items[i].HeightCm,
			Quantity:     items[i].Quantity,
			DesignSource: string(items[i].DesignSource),
		})
	}
	return out
}

// discountLabel implements §28.7's "nama apa yang ditampilkan" rule ONCE
// (order module), so every consumer (POS struk, invoice PDF, admin detail)
// gets a ready-to-print string instead of re-deriving it: order's own
// discount_name_snapshot when there is one, "Diskon" for a manual discount
// (which has no name), "" when discount_amount is 0 (caller must NOT print a
// "Diskon Rp 0" line — §28.7).
func discountLabel(o *model.Order) string {
	if o.DiscountAmount <= 0 {
		return ""
	}
	if o.DiscountNameSnapshot != nil && *o.DiscountNameSnapshot != "" {
		return *o.DiscountNameSnapshot
	}
	return "Diskon"
}

func orderToSummary(o *model.Order) *orderapi.OrderSummary {
	sum := &orderapi.OrderSummary{
		ID:                     o.ID,
		Resi:                   o.Resi,
		CustomerID:             o.CustomerID,
		Status:                 string(o.Status),
		Total:                  o.Total,
		MetodeAmbil:            string(o.MetodeAmbil),
		Channel:                string(o.Channel),
		DesignSource:           string(o.DesignSource),
		CreatedBy:              o.CreatedBy,
		CreatedAt:              o.CreatedAt,
		Items:                  itemsToView(o.Items),
		Subtotal:               o.Subtotal,
		DiscountAmount:         o.DiscountAmount,
		DiscountLabel:          discountLabel(o),
		ShippingCost:           o.ShippingCost,
		ShippingRecipientPhone: o.ShippingRecipientPhone,
	}
	if o.MetodeBayar != nil {
		sum.MetodeBayar = string(*o.MetodeBayar)
	}
	if o.DesignApprovalMode != nil {
		s := string(*o.DesignApprovalMode)
		sum.DesignApprovalMode = &s
	}
	return sum
}

// MarkPendingVerification implements orderapi.OrderCommandService.
// Valid from menunggu_pembayaran (first upload) or ditolak (re-upload).
func (s *Service) MarkPendingVerification(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	allowed := []string{string(state.MenungguPembayaran), string(state.Ditolak)}
	return s.commandTransition(ctx, orderID, allowed, state.MenungguVerifikasi, "", actorID, note)
}

// MarkDibayar implements orderapi.OrderCommandService.
// Valid from menunggu_verifikasi (proof approved) or menunggu_pembayaran (POS).
func (s *Service) MarkDibayar(ctx context.Context, orderID uuid.UUID, metodeBayar string, actorID *uuid.UUID, note string) error {
	if metodeBayar == "" {
		return fmt.Errorf("metode_bayar wajib diisi saat mark dibayar")
	}
	allowed := []string{string(state.MenungguVerifikasi), string(state.MenungguPembayaran)}
	return s.commandTransition(ctx, orderID, allowed, state.Dibayar, metodeBayar, actorID, note)
}

// MarkDitolak implements orderapi.OrderCommandService.
// Valid only from menunggu_verifikasi.
func (s *Service) MarkDitolak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("reject reason wajib diisi")
	}
	return s.commandTransition(ctx, orderID, []string{string(state.MenungguVerifikasi)},
		state.Ditolak, "", actorID, reason)
}

// MarkDesainDikerjakan implements orderapi.OrderCommandService.
// Valid dari dibayar (initial) atau menunggu_approval_desain (customer minta revisi).
func (s *Service) MarkDesainDikerjakan(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	allowed := []string{string(state.Dibayar), string(state.MenungguApprovalDesain)}
	return s.commandTransition(ctx, orderID, allowed, state.DesainDikerjakan, "", actorID, note)
}

// MarkMenungguApprovalDesain implements orderapi.OrderCommandService.
// Valid dari desain_dikerjakan (staff selesai draft, kirim ke customer).
func (s *Service) MarkMenungguApprovalDesain(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	allowed := []string{string(state.DesainDikerjakan)}
	return s.commandTransition(ctx, orderID, allowed, state.MenungguApprovalDesain, "", actorID, note)
}

// MarkDesainDiverifikasi implements orderapi.OrderCommandService.
// Valid dari:
//   - dibayar                 (upload path)
//   - menunggu_approval_desain (request path — customer approve)
//   - desain_dikerjakan       (walk-in POS — §11 "Disetujui Langsung")
func (s *Service) MarkDesainDiverifikasi(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	allowed := []string{
		string(state.Dibayar),
		string(state.MenungguApprovalDesain),
		string(state.DesainDikerjakan),
	}
	return s.commandTransition(ctx, orderID, allowed, state.DesainDiverifikasi, "", actorID, note)
}

// MarkProsesCetak — staff mulai cetak. Valid dari desain_diverifikasi.
func (s *Service) MarkProsesCetak(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	return s.commandTransition(ctx, orderID,
		[]string{string(state.DesainDiverifikasi)},
		state.ProsesCetak, "", actorID, note)
}

// MarkQC — hasil masuk QC. Valid dari proses_cetak.
func (s *Service) MarkQC(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	return s.commandTransition(ctx, orderID,
		[]string{string(state.ProsesCetak)},
		state.QC, "", actorID, note)
}

// MarkSiapKirimAtauAmbil — lulus QC. Auto-branch dari order.metode_ambil:
// kirim → siap_kirim, pickup → siap_ambil. Valid dari qc.
func (s *Service) MarkSiapKirimAtauAmbil(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orderapi.ErrOrderNotFound
		}
		return fmt.Errorf("mark siap-kirim/ambil: lookup order: %w", err)
	}
	var target state.Status
	switch o.MetodeAmbil {
	case model.MetodeAmbilKirim:
		target = state.SiapKirim
	case model.MetodeAmbilPickup:
		target = state.SiapAmbil
	default:
		return fmt.Errorf("mark siap-kirim/ambil: metode_ambil invalid: %q", o.MetodeAmbil)
	}
	return s.commandTransition(ctx, orderID,
		[]string{string(state.QC)},
		target, "", actorID, note)
}

// MarkDikirim — paket diserahkan ekspedisi. Wajib metode_ambil=kirim +
// courier/tracking non-empty. Valid dari siap_kirim.
func (s *Service) MarkDikirim(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, courier, trackingNumber, note string) error {
	courier = strings.TrimSpace(courier)
	trackingNumber = strings.TrimSpace(trackingNumber)
	if courier == "" || trackingNumber == "" {
		return orderapi.ErrShippingTrackingRequired
	}
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orderapi.ErrOrderNotFound
		}
		return fmt.Errorf("mark dikirim: lookup order: %w", err)
	}
	if o.MetodeAmbil != model.MetodeAmbilKirim {
		return orderapi.ErrDikirimOnPickup
	}
	params := repository.AdvanceStatusParams{
		OrderID:                orderID,
		AllowedFromStatuses:    []string{string(state.SiapKirim)},
		NewStatus:              string(state.Dikirim),
		ShippingCourier:        courier,
		ShippingTrackingNumber: trackingNumber,
		ChangedBy:              actorID,
	}
	if note != "" {
		n := note
		params.Note = &n
	}
	if err := s.orders.AdvanceStatus(ctx, params); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return orderapi.ErrOrderNotFound
		case errors.Is(err, repository.ErrStaleState):
			return orderapi.ErrOrderStateChanged
		default:
			return fmt.Errorf("mark dikirim: %w", err)
		}
	}
	return nil
}

// MarkSelesai — order tuntas. Valid dari dikirim (kirim path) atau siap_ambil (pickup).
func (s *Service) MarkSelesai(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, note string) error {
	allowed := []string{string(state.Dikirim), string(state.SiapAmbil)}
	return s.commandTransition(ctx, orderID, allowed, state.Selesai, "", actorID, note)
}

// MarkDibatalkan — cancel order dari pre-cetak state (§4). Reason wajib
// (masuk log status_history + audit). Post-cetak state tidak boleh cancel —
// harus diselesaikan atau refund out-of-band.
//
// Notification WA: TODO nanti tambah kind order_cancelled kalau perlu; sekarang
// cukup log saja (customer biasanya diberi tahu tatap muka / manual).
func (s *Service) MarkDibatalkan(ctx context.Context, orderID uuid.UUID, actorID *uuid.UUID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("alasan pembatalan wajib diisi")
	}
	// preCetakStates dari state machine — sama dengan §4.
	allowed := []string{
		string(state.OrderMasuk),
		string(state.MenungguOngkir),
		string(state.MenungguPembayaran),
		string(state.MenungguVerifikasi),
		string(state.Ditolak),
		string(state.Dibayar),
		string(state.DesainDikerjakan),
		string(state.MenungguApprovalDesain),
		string(state.DesainDiverifikasi),
	}
	return s.commandTransition(ctx, orderID, allowed, state.Dibatalkan, "", actorID, reason)
}

// commandTransition — internal helper shared by the 3 command methods above.
// Maps repository.ErrStaleState → orderapi.ErrOrderStateChanged, ErrNotFound
// → ErrOrderNotFound.
func (s *Service) commandTransition(
	ctx context.Context,
	orderID uuid.UUID,
	allowedFrom []string,
	target state.Status,
	metodeBayar string,
	actorID *uuid.UUID,
	note string,
) error {
	params := repository.AdvanceStatusParams{
		OrderID:             orderID,
		NewStatus:           string(target),
		AllowedFromStatuses: allowedFrom,
		MetodeBayar:         metodeBayar,
		ChangedBy:           actorID,
	}
	if note != "" {
		n := note
		params.Note = &n
	}
	if err := s.orders.AdvanceStatus(ctx, params); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return orderapi.ErrOrderNotFound
		case errors.Is(err, repository.ErrStaleState):
			return orderapi.ErrOrderStateChanged
		default:
			return fmt.Errorf("command transition to %s: %w", target, err)
		}
	}
	return nil
}

// --- helpers ---

func (s *Service) resolveCustomer(ctx context.Context, in CreateOnlineOrderInput) (uuid.UUID, error) {
	if in.CustomerID != nil {
		return *in.CustomerID, nil
	}
	guest, err := s.customers.ResolveOrCreateGuest(ctx, in.GuestPhone, in.GuestName)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve guest: %w", err)
	}
	return guest.UserID, nil
}

func (s *Service) validateOnlineInput(in CreateOnlineOrderInput) error {
	if in.MetodeAmbil != model.MetodeAmbilPickup && in.MetodeAmbil != model.MetodeAmbilKirim {
		return fmt.Errorf("metode_ambil invalid: %q", in.MetodeAmbil)
	}
	if in.MetodeAmbil == model.MetodeAmbilKirim {
		if strings.TrimSpace(in.ShippingAddress) == "" ||
			strings.TrimSpace(in.ShippingRecipientName) == "" ||
			strings.TrimSpace(in.ShippingRecipientPhone) == "" {
			return orderapi.ErrShippingFieldsRequired
		}
	}
	return nil
}

// maxItemsPerOrder / minItemsPerOrder — §32.4: batas jumlah item per order,
// ditolak eksplisit (bukan dibiarkan lewat lalu meledak di tempat lain, mis.
// ratusan panggilan catalog.Quote dalam satu request).
const (
	minItemsPerOrder = 1
	maxItemsPerOrder = 20
)

// validateItemCount implements §32.4's item-count guard — shared by
// CreateOnlineOrder & CreatePOSOrder so the bound lives in exactly one place.
func validateItemCount(n int) error {
	if n < minItemsPerOrder {
		return orderapi.ErrNoItems
	}
	if n > maxItemsPerOrder {
		return orderapi.ErrTooManyItems
	}
	return nil
}

// deriveDesignSource implements §32.1's rule for orders.DesignSource: it is
// a DERIVED summary of order_items.DesignSource, written by the order
// service every time the item list changes — upload only → "upload",
// request only → "request", a mix of both → "mixed". This is the ONLY place
// this rule may live (§22) — every order-creation path (CreateOnlineOrder,
// CreatePOSOrder) MUST funnel through it instead of re-deriving its own copy.
func deriveDesignSource(items []model.OrderItem) model.DesignSource {
	hasUpload, hasRequest := false, false
	for i := range items {
		switch items[i].DesignSource {
		case model.DesignSourceUpload:
			hasUpload = true
		case model.DesignSourceRequest:
			hasRequest = true
		}
	}
	switch {
	case hasUpload && hasRequest:
		return model.DesignSourceMixed
	case hasRequest:
		return model.DesignSourceRequest
	default:
		return model.DesignSourceUpload
	}
}

func (s *Service) createWithResiRetry(ctx context.Context, order *model.Order, items []model.OrderItem, initialStatus state.Status, initialNote *string, changedBy *uuid.UUID) error {
	for attempt := 0; attempt < resiRetryAttempts; attempt++ {
		r, err := resi.Generate()
		if err != nil {
			return fmt.Errorf("generate resi: %w", err)
		}
		order.Resi = r
		hist := &model.OrderStateHistory{
			ToStatus:  initialStatus,
			Note:      initialNote,
			ChangedBy: changedBy,
		}
		err = s.orders.CreateWithHistory(ctx, order, items, hist)
		if err == nil {
			return nil
		}
		if errors.Is(err, repository.ErrResiConflict) {
			continue // retry with a new resi
		}
		return err
	}
	return orderapi.ErrResiCollisionGaveUp
}

func strPtr(s string) *string { return &s }

// CreatePOSOrder implements orderapi.OrderCommandService (§11).
// Atomically create walk-in order langsung di status dibayar. Initial history
// row: from=null, to=dibayar dgn note POS instant payment. Tidak trigger WA
// atau invoice — itu tanggung jawab modul POS (yg lebih tahu context UX).
func (s *Service) CreatePOSOrder(ctx context.Context, in orderapi.POSCreateOrderInput) (*orderapi.OrderSummary, error) {
	// Validate item count (§32.4) & metode_ambil basic shape.
	if err := validateItemCount(len(in.Items)); err != nil {
		return nil, err
	}
	metodeAmbil := model.MetodeAmbil(in.MetodeAmbil)
	if metodeAmbil != model.MetodeAmbilPickup && metodeAmbil != model.MetodeAmbilKirim {
		return nil, fmt.Errorf("metode_ambil invalid: %q", in.MetodeAmbil)
	}
	metodeBayar := model.MetodeBayar(in.MetodeBayar)
	if metodeBayar != model.MetodeBayarCash && metodeBayar != model.MetodeBayarQRISPOS {
		return nil, fmt.Errorf("metode_bayar POS harus cash atau qris_pos, got %q", in.MetodeBayar)
	}
	if in.CustomerID == uuid.Nil {
		return nil, fmt.Errorf("customer_id wajib (resolve dulu via customer service)")
	}
	if in.KasirID == uuid.Nil {
		return nil, fmt.Errorf("kasir_id wajib")
	}

	// Shipping validation
	shippingPhoneNorm := ""
	shippingCost := int64(0)
	if metodeAmbil == model.MetodeAmbilKirim {
		if strings.TrimSpace(in.ShippingAddress) == "" ||
			strings.TrimSpace(in.ShippingRecipientName) == "" ||
			strings.TrimSpace(in.ShippingRecipientPhone) == "" {
			return nil, orderapi.ErrShippingFieldsRequired
		}
		var err error
		shippingPhoneNorm, err = phone.Normalize(in.ShippingRecipientPhone)
		if err != nil {
			return nil, fmt.Errorf("shipping recipient phone: %w", err)
		}
		if in.ShippingCost < 0 {
			return nil, orderapi.ErrInvalidShippingCost
		}
		shippingCost = in.ShippingCost
	}

	// Quote setiap item via catalog (authoritative) + build order_items (§32).
	items, subtotal, err := s.quotePOSItems(ctx, in.Items)
	if err != nil {
		return nil, err
	}

	// Discount (§28.3/§32.3) — resolved AFTER subtotal, BEFORE total. Ongkir
	// tidak pernah didiskon: total = subtotal - discount_amount + shipping_cost.
	discountSnap, err := s.resolveDiscount(ctx, discountapi.ResolveInput{
		DiscountID:   in.DiscountID,
		ManualAmount: in.ManualDiscountAmount,
		Note:         in.DiscountNote,
		Items:        resolveItemsFrom(items),
		Channel:      string(model.ChannelPOS),
		CustomerID:   in.CustomerID,
	})
	if err != nil {
		return nil, err
	}
	total := subtotal - discountSnap.Amount + shippingCost
	if err := applyItemDiscountAllocations(items, discountSnap); err != nil {
		return nil, err
	}

	order := &model.Order{
		CustomerID:   in.CustomerID,
		Channel:      model.ChannelPOS,
		Status:       state.Dibayar, // POS langsung dibayar
		Subtotal:     subtotal,
		MetodeAmbil:  metodeAmbil,
		MetodeBayar:  &metodeBayar,
		DesignSource: deriveDesignSource(items),
		Total:        total,
		CreatedBy:    &in.KasirID,
	}
	applyDiscountSnapshot(order, discountSnap)
	if metodeAmbil == model.MetodeAmbilKirim {
		order.ShippingAddress = strPtr(in.ShippingAddress)
		order.ShippingRecipientName = strPtr(in.ShippingRecipientName)
		order.ShippingRecipientPhone = strPtr(shippingPhoneNorm)
		order.ShippingCost = &shippingCost
	}
	if in.DesignApprovalMode != "" {
		mode := model.DesignApprovalMode(in.DesignApprovalMode)
		if mode != model.DesignApprovalInstantWalkin && mode != model.DesignApprovalAsyncNotify {
			return nil, fmt.Errorf("design_approval_mode invalid: %q", in.DesignApprovalMode)
		}
		order.DesignApprovalMode = &mode
	}
	if in.Notes != "" {
		order.Notes = strPtr(in.Notes)
	}

	initialNote := "POS instant payment (" + in.MetodeBayar + ")"
	kasir := in.KasirID
	if err := s.createWithResiRetry(ctx, order, items, state.Dibayar, &initialNote, &kasir); err != nil {
		return nil, err
	}
	order.Items = items
	return orderToSummary(order), nil
}

// quotePOSItems quotes each requested item via catalog (authoritative
// pricing) and builds the model.OrderItem rows + aggregate subtotal for
// CreatePOSOrder. Mirror of quoteOnlineItems, kept separate because POS
// items carry their own ItemNotes field (§11) that the online form doesn't.
func (s *Service) quotePOSItems(ctx context.Context, in []orderapi.POSOrderItemInput) ([]model.OrderItem, int64, error) {
	items := make([]model.OrderItem, 0, len(in))
	var subtotal int64
	for i, it := range in {
		designSource := model.DesignSource(it.DesignSource)
		if designSource != model.DesignSourceUpload && designSource != model.DesignSourceRequest {
			return nil, 0, fmt.Errorf("item %d: design_source %q: %w", i+1, it.DesignSource, orderapi.ErrInvalidDesignSource)
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		quote, err := s.catalog.Quote(ctx, catalogapi.QuoteRequest{
			ProductID:  it.ProductID,
			MaterialID: it.MaterialID,
			WidthCm:    it.WidthCm,
			HeightCm:   it.HeightCm,
		})
		if err != nil {
			// §22 — dibungkus dengan nomor baris (POS bisa sampai 20 item
			// dalam satu order sekaligus).
			return nil, 0, fmt.Errorf("item %d: quote: %w", i+1, err)
		}
		lineSubtotal := quote.TotalPrice * int64(qty)
		subtotal += lineSubtotal
		item := model.OrderItem{
			LineNo:               i + 1,
			ProductID:            &quote.ProductID,
			ProductNameSnapshot:  quote.ProductName,
			MaterialID:           &quote.MaterialID,
			MaterialNameSnapshot: quote.MaterialName,
			PricingTypeSnapshot:  string(quote.PricingType),
			WidthCm:              it.WidthCm,
			HeightCm:             it.HeightCm,
			Quantity:             qty,
			UnitPrice:            quote.TotalPrice,
			Subtotal:             lineSubtotal,
			DesignSource:         designSource,
			ChargeableM2:         quote.ChargeableM2 * float64(qty),
		}
		if it.DesignBrief != "" {
			item.DesignBrief = strPtr(it.DesignBrief)
		}
		if it.ItemNotes != "" {
			item.ItemNotes = strPtr(it.ItemNotes)
		}
		items = append(items, item)
	}
	return items, subtotal, nil
}

// resolveItemsFrom projects quoted order_items into the discountapi.ResolveItem
// shape the discount module's Resolver expects (§32.3) — order module never
// imports discount/model, only discountapi.
func resolveItemsFrom(items []model.OrderItem) []discountapi.ResolveItem {
	out := make([]discountapi.ResolveItem, len(items))
	for i := range items {
		pid := uuid.Nil
		if items[i].ProductID != nil {
			pid = *items[i].ProductID
		}
		chargeableM2 := items[i].ChargeableM2
		if chargeableM2 <= 0 && items[i].PricingTypeSnapshot == "per_m2" && items[i].WidthCm > 0 && items[i].HeightCm > 0 {
			wM := float64(items[i].WidthCm) / 100.0
			hM := float64(items[i].HeightCm) / 100.0
			qty := items[i].Quantity
			if qty <= 0 {
				qty = 1
			}
			chargeableM2 = wM * hM * float64(qty)
		}
		out[i] = discountapi.ResolveItem{
			LineNo:       items[i].LineNo,
			ProductID:    pid,
			Subtotal:     items[i].Subtotal,
			PricingType:  items[i].PricingTypeSnapshot,
			WidthCm:      items[i].WidthCm,
			HeightCm:     items[i].HeightCm,
			Quantity:     items[i].Quantity,
			ChargeableM2: chargeableM2,
		}
	}
	return out
}

// applyItemDiscountAllocations copies discountapi.Snapshot.Allocations
// (§32.3, largest-remainder method) onto each order_item's DiscountAmount —
// a no-op when snap describes "no discount" (Amount 0, Allocations empty).
//
// Before copying anything, it enforces §32.2's invariant ITSELF, on the
// order module's own side of the boundary — it does NOT just trust that
// discount/service/calc.go got the math right: if Allocations is empty while
// Amount > 0, or Σ Allocations.Amount != Amount, that is EXACTLY the silent
// failure class §22 forbids (rekap/struk would show a discount_amount that
// no item actually reflects), so it refuses to save the order at all
// (orderapi.ErrDiscountAllocationMismatch) instead of writing whatever the
// resolver handed back.
func applyItemDiscountAllocations(items []model.OrderItem, snap *discountapi.Snapshot) error {
	if len(snap.Allocations) == 0 {
		if snap.Amount != 0 {
			return fmt.Errorf("alokasi diskon: discount_amount %d tapi resolver tidak mengembalikan alokasi apa pun: %w",
				snap.Amount, orderapi.ErrDiscountAllocationMismatch)
		}
		return nil
	}
	var reported int64
	byLine := make(map[int]int64, len(snap.Allocations))
	for _, a := range snap.Allocations {
		byLine[a.LineNo] = a.Amount
		reported += a.Amount
	}
	if reported != snap.Amount {
		return fmt.Errorf("alokasi diskon: Σ alokasi %d != discount_amount %d: %w",
			reported, snap.Amount, orderapi.ErrDiscountAllocationMismatch)
	}
	// §22 review #4 — the guard above only proves the RESOLVER's own report
	// sums correctly; it does NOT prove any allocation actually landed on a
	// real item. A resolver returning LineNo values that don't match any
	// items[i].LineNo (e.g. 0-based instead of 1-based) would pass the check
	// above while leaving every items[i].DiscountAmount at 0. Track what was
	// actually APPLIED and compare that against snap.Amount instead.
	var applied int64
	for i := range items {
		if amt, ok := byLine[items[i].LineNo]; ok {
			items[i].DiscountAmount = amt
			applied += amt
		}
	}
	if applied != snap.Amount {
		return fmt.Errorf("alokasi diskon: Σ alokasi yang benar-benar diterapkan ke item %d != discount_amount %d (LineNo dari resolver tidak cocok item manapun): %w",
			applied, snap.Amount, orderapi.ErrDiscountAllocationMismatch)
	}
	return nil
}

// resolveDiscount is the single gate every order-creation path (currently
// just CreatePOSOrder — CreateOnlineOrder never carries a discount, §28)
// funnels through before computing `total`. Refuses explicitly
// (orderapi.ErrDiscountUnavailable) rather than silently skipping the
// discount when a discount/manual-discount was requested but s.discount was
// never wired (§22 no-silent-stub) — a genuinely "no discount requested"
// input (both DiscountID nil and ManualAmount<=0) is fine even without a
// resolver, since discountapi.Resolver itself would just return a zero
// Snapshot for that case anyway.
func (s *Service) resolveDiscount(ctx context.Context, in discountapi.ResolveInput) (*discountapi.Snapshot, error) {
	if s.discount == nil {
		if in.DiscountID != nil || in.ManualAmount > 0 {
			return nil, orderapi.ErrDiscountUnavailable
		}
		return &discountapi.Snapshot{}, nil
	}
	return s.discount.ResolveForOrder(ctx, in)
}

// applyDiscountSnapshot copies a resolved discountapi.Snapshot onto the
// order's own snapshot columns (§28.2 lapis 1) — a no-op (every field stays
// at its zero value) when snap describes "no discount" (Type == "").
// DiscountValueSnapshot is deliberately left nil for manual discounts (the
// column is documented as NULL for that case in migration 000027 — a manual
// discount has no meaningful "value", only an Amount).
func applyDiscountSnapshot(o *model.Order, snap *discountapi.Snapshot) {
	o.DiscountAmount = snap.Amount
	if snap.DiscountID != nil {
		id := *snap.DiscountID
		o.DiscountID = &id
	}
	if snap.Code != "" {
		o.DiscountCodeSnapshot = strPtr(snap.Code)
	}
	if snap.Name != "" {
		o.DiscountNameSnapshot = strPtr(snap.Name)
	}
	if snap.Type != "" {
		o.DiscountTypeSnapshot = strPtr(snap.Type)
		// "manual" — discountapi.Snapshot's documented sentinel value for a
		// cashier-entered manual discount (order module intentionally does
		// NOT import discount/model — only discountapi, its public contract).
		if snap.Type != "manual" {
			v := snap.Value
			o.DiscountValueSnapshot = &v
		}
	}
	if snap.Note != "" {
		o.DiscountNote = strPtr(snap.Note)
	}
}

// ListPOSOrdersByDate implements orderapi.OrderCommandService (§11 rekonsiliasi).
// dateWIB: instance dgn hari + zona WIB; service hitung window [00:00 WIB, next 00:00 WIB).
func (s *Service) ListPOSOrdersByDate(ctx context.Context, dateWIB time.Time) ([]orderapi.OrderSummary, error) {
	// Normalize ke 00:00 WIB pada hari yg diminta.
	loc := jakartaTZ()
	local := dateWIB.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	rows, err := s.orders.ListPOSByDateRange(ctx, start.UTC(), end.UTC())
	if err != nil {
		return nil, fmt.Errorf("list POS orders: %w", err)
	}
	out := make([]orderapi.OrderSummary, len(rows))
	for i := range rows {
		out[i] = *orderToSummary(&rows[i])
	}
	return out, nil
}

// jakartaTZ — helper WIB timezone (dupe kecil dari invoice/service — tidak
// bisa cross-import). Fallback fixed offset kalau tzdata tidak tersedia.
func jakartaTZ() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}
