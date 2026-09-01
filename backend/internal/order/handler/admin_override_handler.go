package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/service"
	"github.com/rajaku-printing/backend/internal/order/state"
)

// ---- Request DTOs ----

// editOrderItemRequest — satu baris di editOrderRequest.Items (§32.9).
// ID nil = baris baru (product_id/material_id/design_source WAJIB diisi,
// unit_price diabaikan — selalu di-Quote ulang lewat catalog di service).
// ID non-nil = edit baris existing (product_id/material_id/design_source/
// design_brief WAJIB dibiarkan kosong — service menolak eksplisit kalau
// diisi, §32.9).
type editOrderItemRequest struct {
	ID         *uuid.UUID `json:"id"`
	ProductID  *uuid.UUID `json:"product_id"`
	MaterialID *uuid.UUID `json:"material_id"`
	WidthCm    int        `json:"width_cm"   binding:"required,gt=0"`
	HeightCm   int        `json:"height_cm"  binding:"required,gt=0"`
	Quantity   int        `json:"quantity"   binding:"required,gt=0"`
	UnitPrice  int64      `json:"unit_price" binding:"omitempty,gte=0"`
	ItemNotes  *string    `json:"item_notes" binding:"omitempty,max=1000"`

	DesignSource string `json:"design_source" binding:"omitempty,oneof=upload request"`
	DesignBrief  string `json:"design_brief"  binding:"omitempty,max=2000"`
}

// editOrderRequest — PATCH /admin/orders/:resi body. Items nil = daftar item
// tidak disentuh sama sekali; Items dikirim (walau isinya sama persis dengan
// yang sudah ada) = ganti SELURUH daftar item pesanan dengan ini (§32.9) —
// 1..20 baris, sama seperti batas create order (§32.4).
type editOrderRequest struct {
	ShippingRecipientName  *string                 `json:"shipping_recipient_name"  binding:"omitempty,min=1,max=255"`
	ShippingRecipientPhone *string                 `json:"shipping_recipient_phone" binding:"omitempty,min=8,max=20"`
	ShippingAddress        *string                 `json:"shipping_address"         binding:"omitempty,max=1000"`
	Note                   *string                 `json:"note"                     binding:"omitempty,max=1000"`
	Subtotal               *int64                  `json:"subtotal"                 binding:"omitempty,gte=0"`
	ShippingCost           *int64                  `json:"shipping_cost"            binding:"omitempty,gte=0"`
	Items                  *[]editOrderItemRequest `json:"items"                    binding:"omitempty,min=1,max=20,dive"`
	Reason                 string                  `json:"reason"                   binding:"omitempty,max=500"`
}

type overrideStatusRequest struct {
	ToStatus string `json:"to_status" binding:"required"`
	Reason   string `json:"reason"    binding:"required,min=10,max=500"`
}

type deleteOrderRequest struct {
	Reason string `json:"reason" binding:"required,min=10,max=500"`
}

// ---- Response DTOs ----

type auditLogRowResponse struct {
	ID          string                          `json:"id"`
	ActorUserID string                          `json:"actor_user_id"`
	Action      string                          `json:"action"`
	EntityType  string                          `json:"entity_type"`
	EntityID    string                          `json:"entity_id"`
	EntityLabel string                          `json:"entity_label,omitempty"`
	Changes     map[string]auditFieldChangeResp `json:"changes,omitempty"`
	Reason      string                          `json:"reason"`
	CreatedAt   string                          `json:"created_at"`
}

type auditFieldChangeResp struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// PATCH /admin/orders/:resi — super admin koreksi data pesanan.
// Requires: RequireUserType(staff) + RequirePermission("order.edit").
func (h *Handler) AdminEditOrder(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req editOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	in := service.EditOrderInput{
		ShippingRecipientName:  req.ShippingRecipientName,
		ShippingRecipientPhone: req.ShippingRecipientPhone,
		ShippingAddress:        req.ShippingAddress,
		Note:                   req.Note,
		Subtotal:               req.Subtotal,
		ShippingCost:           req.ShippingCost,
	}
	if req.Items != nil {
		items := make([]service.EditOrderItemInput, 0, len(*req.Items))
		for _, it := range *req.Items {
			items = append(items, service.EditOrderItemInput{
				ID:           it.ID,
				ProductID:    it.ProductID,
				MaterialID:   it.MaterialID,
				WidthCm:      it.WidthCm,
				HeightCm:     it.HeightCm,
				Quantity:     it.Quantity,
				UnitPrice:    it.UnitPrice,
				ItemNotes:    it.ItemNotes,
				DesignSource: it.DesignSource,
				DesignBrief:  it.DesignBrief,
			})
		}
		in.Items = &items
	}
	got, err := h.svc.EditOrder(c.Request.Context(), resi, id.UserID, in, req.Reason)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(got))
}

// POST /admin/orders/:resi/override-status — super admin paksa ubah status
// ke status manapun yang dikenal, melewati validasi transisi normal.
// Requires: RequireUserType(staff) + RequirePermission("order.override_status").
func (h *Handler) AdminOverrideStatus(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req overrideStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	toStatus := state.Status(req.ToStatus)
	if !state.IsKnown(toStatus) {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "to_status tidak dikenal")
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	got, err := h.svc.OverrideStatus(c.Request.Context(), resi, id.UserID, toStatus, req.Reason)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(got))
}

// DELETE /admin/orders/:resi — super admin "hapus" pesanan (SOFT delete —
// data tetap ada untuk rekap, hanya disembunyikan dari tampilan normal).
// Requires: RequireUserType(staff) + RequirePermission("order.delete").
func (h *Handler) AdminDeleteOrder(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req deleteOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	if err := h.svc.SoftDeleteOrder(c.Request.Context(), resi, id.UserID, req.Reason); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"resi": resi, "deleted": true})
}

// GET /admin/audit-log — riwayat aksi super admin (edit/override/hapus).
// Query params: entity_id (opsional, UUID), limit (default 50, max 200).
// Requires: RequireUserType(staff) + RequirePermission("audit.view").
func (h *Handler) AdminListAuditLog(c *gin.Context) {
	var entityID *uuid.UUID
	if raw := c.Query("entity_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "entity_id bukan UUID valid")
			return
		}
		entityID = &parsed
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "limit harus berupa angka")
			return
		}
		limit = n
	}

	rows, err := h.svc.ListAuditLog(c.Request.Context(), entityID, limit)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	items := make([]auditLogRowResponse, 0, len(rows))
	for _, r := range rows {
		item := auditLogRowResponse{
			ID:          r.ID,
			ActorUserID: r.ActorUserID,
			Action:      r.Action,
			EntityType:  r.EntityType,
			EntityID:    r.EntityID,
			EntityLabel: r.EntityLabel,
			Reason:      r.Reason,
			CreatedAt:   r.CreatedAt,
		}
		if len(r.Changes) > 0 {
			item.Changes = make(map[string]auditFieldChangeResp, len(r.Changes))
			for k, v := range r.Changes {
				item.Changes[k] = auditFieldChangeResp{From: v.From, To: v.To}
			}
		}
		items = append(items, item)
	}
	httpx.OK(c, gin.H{"items": items})
}
