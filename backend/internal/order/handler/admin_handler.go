package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/service"
)

// GET /admin/orders — paginated list with filters.
// Query params: status, channel, page, page_size.
// Requires: RequireUserType(staff) + RequirePermission("order.view").
func (h *Handler) AdminList(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	in := service.AdminListInput{
		Status:   c.Query("status"),
		Channel:  c.Query("channel"),
		Page:     page,
		PageSize: pageSize,
	}
	res, err := h.svc.ListForAdmin(c.Request.Context(), in)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	items := make([]orderResponse, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toOrderResponse(&res.Items[i]))
	}
	httpx.OK(c, gin.H{
		"items":     items,
		"total":     res.Total,
		"page":      res.Page,
		"page_size": res.PageSize,
	})
}

// GET /admin/orders/:resi — full detail untuk halaman /admin/order/:resi.
// Return order + customer info + history rows.
// Requires: RequireUserType(staff) + RequirePermission("order.view").
func (h *Handler) AdminGetDetail(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	order, detail, err := h.svc.AdminGetDetail(c.Request.Context(), resi)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	// Compose: order response + detail (customer + history).
	base := toOrderResponse(order)
	httpx.OK(c, gin.H{
		"order":    base,
		"customer": detail.CustomerInfo,
		"history":  detail.History,
	})
}

// POST /admin/orders/:resi/shipping-cost — kirim only.
// Requires: RequireUserType(staff) + RequirePermission("shipping.set_cost").
func (h *Handler) AdminSetShippingCost(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req setShippingCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	got, err := h.svc.SetShippingCost(c.Request.Context(), service.SetShippingCostInput{
		Resi:         resi,
		ShippingCost: req.ShippingCost,
		StaffID:      id.UserID,
		Note:         req.Note,
	})
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(got))
}

// POST /admin/orders/:resi/confirm-pickup — pickup only.
// Requires: RequireUserType(staff) + RequirePermission("order.update_status").
func (h *Handler) AdminConfirmPickup(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req confirmPickupRequest
	// Body optional — bind only if present, ignore missing body.
	_ = c.ShouldBindJSON(&req)

	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	got, err := h.svc.ConfirmPickupTotal(c.Request.Context(), resi, id.UserID, req.Note)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(got))
}

// POST /admin/orders/:resi/cancel — cancel order (§4, hanya dari pre-cetak state).
// Requires: RequireUserType(staff) + RequirePermission("order.cancel").
func (h *Handler) AdminCancel(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}
	var req cancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	// Lookup order dulu untuk dapat ID (service pakai ID, bukan resi).
	sum, err := h.svc.FindSummaryByResi(c.Request.Context(), resi)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	staffID := id.UserID
	if err := h.svc.MarkDibatalkan(c.Request.Context(), sum.ID, &staffID, req.Reason); err != nil {
		h.mapAdminErr(c, err)
		return
	}
	// Re-fetch full order untuk response.
	got, err := h.svc.GetByResiForOwner(c.Request.Context(), resi, id)
	if err != nil {
		h.mapAdminErr(c, err)
		return
	}
	httpx.OK(c, toOrderResponse(got))
}

// mapAdminErr routes admin-specific transition errors to HTTP; delegates the
// rest to the shared mapErr for consistent handling.
func (h *Handler) mapAdminErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, orderapi.ErrShippingCostOnPickup):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable,
			"metode ambil order ini pickup — tidak bisa diisi ongkir")
	case errors.Is(err, orderapi.ErrConfirmPickupOnKirim):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable,
			"endpoint ini khusus order pickup, gunakan set-shipping-cost untuk kirim")
	case errors.Is(err, orderapi.ErrInvalidTransition):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"status order tidak boleh ditransisi dari state saat ini")
	case errors.Is(err, orderapi.ErrOrderStateChanged):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"status order berubah bersamaan, refresh dan coba lagi")
	case errors.Is(err, orderapi.ErrInvalidShippingCost):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"shipping_cost harus >= 0")
	default:
		h.mapErr(c, err)
	}
}
