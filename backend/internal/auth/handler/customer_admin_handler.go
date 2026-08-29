// Handler untuk /admin/customers, /admin/customers-export — fitur
// "Manajemen Pelanggan" (admin panel). Terpisah dari AdminHandler (§22 anti
// god-struct) — pola sama seperti membership/handler yang punya Handler
// sendiri di luar AdminHandler.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/auth/service"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/pkg/phone"
)

// CustomerAdminHandler — HTTP-facing bundle untuk endpoint admin pelanggan.
type CustomerAdminHandler struct {
	svc *service.CustomerAdminService
}

func NewCustomerAdminHandler(svc *service.CustomerAdminService) *CustomerAdminHandler {
	return &CustomerAdminHandler{svc: svc}
}

// GET /admin/customers?q=&customer_type=&is_active=&membership_status=&page=&per_page=
func (h *CustomerAdminHandler) ListCustomers(c *gin.Context) {
	in, err := parseListCustomerQuery(c)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	res, err := h.svc.ListCustomers(c.Request.Context(), *in)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// GET /admin/customers/:id
func (h *CustomerAdminHandler) GetCustomer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	res, err := h.svc.GetCustomer(c.Request.Context(), id)
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

type updateCustomerBody struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

// PATCH /admin/customers/:id
func (h *CustomerAdminHandler) UpdateCustomer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body updateCustomerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	actorID, ok := callerID(c)
	if !ok {
		return
	}
	res, err := h.svc.UpdateCustomer(c.Request.Context(), service.UpdateCustomerInput{
		CustomerID: id, Name: body.Name, Email: body.Email, Phone: body.Phone, ActorID: actorID,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

type setCustomerActiveBody struct {
	Reason string `json:"reason"`
}

// POST /admin/customers/:id/deactivate — BATAS: tidak mencabut access token
// yang sudah terbit (JWT stateless, maks TTL 24 jam) — hanya menolak login &
// order BARU berikutnya. Lihat doc-comment service.SetCustomerActive.
func (h *CustomerAdminHandler) DeactivateCustomer(c *gin.Context) {
	h.setActive(c, false)
}

// POST /admin/customers/:id/activate
func (h *CustomerAdminHandler) ActivateCustomer(c *gin.Context) {
	h.setActive(c, true)
}

func (h *CustomerAdminHandler) setActive(c *gin.Context, active bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid id")
		return
	}
	var body setCustomerActiveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "invalid json body")
		return
	}
	actorID, ok := callerID(c)
	if !ok {
		return
	}
	res, err := h.svc.SetCustomerActive(c.Request.Context(), service.SetActiveInput{
		CustomerID: id, Active: active, Reason: body.Reason, ActorID: actorID,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// csvHeaderOnFirstWriteWriter defers setting the CSV response headers until
// the FIRST byte is actually about to be written to the client (temuan
// review #3). Setting Content-Type/Content-Disposition eagerly — BEFORE
// calling the service — meant a service-level rejection (mis.
// ErrExportTooLarge, or one of the validation sentinels from temuan #5)
// still downloaded as a file named "pelanggan.csv" containing the JSON error
// envelope: the browser had already committed to treating the response as a
// CSV attachment by the time the error surfaced, so an admin sees "corrupt
// download" instead of "persempit filternya".
type csvHeaderOnFirstWriteWriter struct {
	c       *gin.Context
	written bool
}

func (w *csvHeaderOnFirstWriteWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.c.Header("Content-Type", "text/csv")
		w.c.Header("Content-Disposition", `attachment; filename="pelanggan.csv"`)
		w.written = true
	}
	return w.c.Writer.Write(p)
}

// GET /admin/customers-export?<filter sama dengan list>
func (h *CustomerAdminHandler) ExportCustomers(c *gin.Context) {
	in, err := parseListCustomerQuery(c)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}

	w := &csvHeaderOnFirstWriteWriter{c: c}
	if err := h.svc.ExportCustomers(c.Request.Context(), *in, w); err != nil {
		if !w.written {
			// Nothing written to the client yet (filter/limit validation
			// always happens before the CSV header row, see
			// service.ExportCustomers) — still safe to answer with a proper
			// JSON error envelope.
			h.mapErr(c, err)
			return
		}
		// Sekali penulisan dimulai, kegagalan di tengah stream tidak bisa
		// lagi dijawab dengan envelope JSON (pola sama
		// order/handler/recap_handler.go) — dicatat via structured log,
		// bukan diabaikan tanpa jejak.
		log.Ctx(c.Request.Context()).Error().Err(err).
			Msg("customer export: stream aborted mid-write, client received a truncated CSV")
		return
	}
}

// parseListCustomerQuery — shared query-string parsing untuk List & Export.
func parseListCustomerQuery(c *gin.Context) (*service.ListCustomerInput, error) {
	page, err := httpx.ParseIntQuery(c, "page", 1)
	if err != nil {
		return nil, err
	}
	perPage, err := httpx.ParseIntQuery(c, "per_page", 0)
	if err != nil {
		return nil, err
	}
	in := &service.ListCustomerInput{
		Q:                c.Query("q"),
		CustomerType:     c.Query("customer_type"),
		MembershipStatus: c.Query("membership_status"),
		Page:             page,
		PerPage:          perPage,
	}
	if raw := c.Query("is_active"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, errors.New("is_active harus boolean (true/false)")
		}
		in.IsActive = &b
	}
	return in, nil
}

func (h *CustomerAdminHandler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authapi.ErrCustomerNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	// Kode error dibedakan per field (bukan CONFLICT generik) supaya frontend
	// bisa menaruh pesannya tepat di bawah field yang bentrok tanpa menebak
	// dari teks pesan Indonesia — teks bisa diperbaiki kapan saja, kode tidak.
	// CodePhoneAlreadyUsed/CodeEmailAlreadyUsed sudah dipakai jalur registrasi
	// & phone-claim di modul ini, jadi frontend menangani satu kosakata saja.
	case errors.Is(err, service.ErrPhoneTaken):
		httpx.Error(c, http.StatusConflict, httpx.CodePhoneAlreadyUsed, err.Error())
	case errors.Is(err, service.ErrEmailTaken):
		httpx.Error(c, http.StatusConflict, httpx.CodeEmailAlreadyUsed, err.Error())
	case errors.Is(err, service.ErrCustomerStatusUnchanged):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, service.ErrInvalidCustomerFilter), errors.Is(err, service.ErrCustomerNameRequired),
		errors.Is(err, service.ErrReasonRequired), errors.Is(err, service.ErrExportTooLarge),
		errors.Is(err, service.ErrEmailRequiredForRegistered), errors.Is(err, service.ErrPhoneRequiredForGuest),
		errors.Is(err, service.ErrCustomerUpdateInvalid),
		errors.Is(err, phone.ErrEmpty), errors.Is(err, phone.ErrContainsLetters),
		errors.Is(err, phone.ErrInvalidPrefix), errors.Is(err, phone.ErrInvalidLength):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("customer admin handler: unmapped error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
	}
}
