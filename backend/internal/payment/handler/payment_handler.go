package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/auth/authapi"
	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	paymodel "github.com/rajaku-printing/backend/internal/payment/model"
	"github.com/rajaku-printing/backend/internal/payment/paymentapi"
	"github.com/rajaku-printing/backend/internal/payment/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// POST /orders/:resi/payment-proof
// Content-Type: multipart/form-data
// Form fields:
//
//	file            (required)
//	metode_bayar    (required, "transfer" | "qris")
//	amount_claimed  (optional, integer)
//
// Auth: RequireAuth (customer OR staff). Staff bypasses ownership check.
func (h *Handler) UploadProof(c *gin.Context) {
	resi := c.Param("resi")
	if resi == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeBadRequest, "resi required")
		return
	}

	id, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || id == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}

	metode := c.PostForm("metode_bayar")
	if metode != string(paymodel.MetodeBayarTransfer) && metode != string(paymodel.MetodeBayarQRIS) {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"metode_bayar wajib 'transfer' atau 'qris'")
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "field 'file' wajib")
		return
	}
	if fh.Size <= 0 {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file kosong")
		return
	}
	f, err := fh.Open()
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("open uploaded file")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "gagal buka file upload")
		return
	}
	defer f.Close()

	var amountClaimed *int64
	if raw := c.PostForm("amount_claimed"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
				"amount_claimed harus bilangan bulat >= 0")
			return
		}
		amountClaimed = &n
	}

	// The mime type from multipart is Content-Type header set by client; browsers
	// generally send accurate values but we still re-check the allowlist in service.
	mime := fh.Header.Get("Content-Type")

	proof, err := h.svc.UploadProof(c.Request.Context(), service.UploadProofInput{
		Resi:          resi,
		CallerID:      id.UserID,
		IsStaff:       id.UserType == authapi.UserTypeStaff,
		MetodeBayar:   paymodel.MetodeBayar(metode),
		AmountClaimed: amountClaimed,
		FileReader:    f,
		FileSize:      fh.Size,
		OriginalName:  fh.Filename,
		MimeType:      mime,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.Created(c, toProofResponse(proof))
}

// GET /admin/payment-proofs?status=pending&order_id=…&page=…&page_size=…
// Auth: staff + "payment.verify".
func (h *Handler) AdminList(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	in := service.ListInput{
		Status:   paymodel.ProofStatus(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	}
	if raw := c.Query("order_id"); raw != "" {
		oid, err := uuid.Parse(raw)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "order_id invalid uuid")
			return
		}
		in.OrderID = &oid
	}
	res, err := h.svc.ListProofs(c.Request.Context(), in)
	if err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("list proofs")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	items := make([]proofResponse, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toProofResponse(&res.Items[i]))
	}
	httpx.OK(c, gin.H{
		"items":     items,
		"total":     res.Total,
		"page":      res.Page,
		"page_size": res.PageSize,
	})
}

// GET /payment-proofs/:id/file — stream the proof file. Auth required.
//
//	Staff: can view any proof.
//	Customer: only proofs on their own orders.
//	?download=1 forces Content-Disposition: attachment (default: inline so
//	browsers preview images/PDF).
func (h *Handler) GetFile(c *gin.Context) {
	proofID, ok := parseProofID(c)
	if !ok {
		return
	}
	caller, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || caller == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	handle, err := h.svc.GetProofFile(c.Request.Context(), proofID, caller.UserID,
		caller.UserType == authapi.UserTypeStaff)
	if err != nil {
		h.mapErr(c, err)
		return
	}

	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	// Quote original filename to survive commas / spaces in HTTP header parsing.
	c.Header("Content-Type", handle.Proof.FileMimeType)
	c.Header("Content-Disposition", disposition+`; filename="`+sanitizeHeaderFilename(handle.Proof.FileOriginalName)+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	// Payment proofs are sensitive — prevent shared caches from storing them.
	c.Header("Cache-Control", "private, no-store")

	// http.ServeFile handles Range/If-Modified-Since correctly and streams
	// without loading the whole file into memory.
	c.File(handle.AbsPath)
}

// sanitizeHeaderFilename strips characters that would break the Content-Disposition
// filename= token (quotes, CRLF, control chars). Not a security boundary — the
// filename was already sanitized once on upload; this is defensive re-quoting.
func sanitizeHeaderFilename(name string) string {
	var b []byte
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '"' || c == '\\' || c == '\r' || c == '\n' || c < 0x20 {
			b = append(b, '_')
			continue
		}
		b = append(b, c)
	}
	if len(b) == 0 {
		return "proof"
	}
	return string(b)
}

// POST /admin/payment-proofs/:id/verify — staff approves proof.
// Auth: staff + "payment.verify".
func (h *Handler) AdminApprove(c *gin.Context) {
	proofID, ok := parseProofID(c)
	if !ok {
		return
	}
	staff, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || staff == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	// Body ignored for now (approveRequest is empty), but we validate it's valid
	// JSON if present so future fields can be added transparently.
	var body approveRequest
	_ = c.ShouldBindJSON(&body)

	proof, err := h.svc.ApproveProof(c.Request.Context(), service.ReviewInput{
		ProofID: proofID,
		StaffID: staff.UserID,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, toProofResponse(proof))
}

// POST /admin/payment-proofs/:id/reject — staff rejects proof, order returns
// to ditolak so customer can re-upload.
// Auth: staff + "payment.reject".
func (h *Handler) AdminReject(c *gin.Context) {
	proofID, ok := parseProofID(c)
	if !ok {
		return
	}
	staff, err := authapi.IdentityFromContext(c.Request.Context())
	if err != nil || staff == nil {
		httpx.Error(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var body rejectRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	proof, err := h.svc.RejectProof(c.Request.Context(), service.ReviewInput{
		ProofID: proofID,
		StaffID: staff.UserID,
		Reason:  body.Reason,
	})
	if err != nil {
		h.mapErr(c, err)
		return
	}
	httpx.OK(c, toProofResponse(proof))
}

func parseProofID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.Param("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "id invalid uuid")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, paymentapi.ErrProofNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "bukti pembayaran tidak ditemukan")
	case errors.Is(err, paymentapi.ErrOrderNotPayable):
		httpx.Error(c, http.StatusUnprocessableEntity, httpx.CodeUnprocessable,
			"status order belum siap menerima bukti pembayaran")
	case errors.Is(err, paymentapi.ErrPendingProofExists):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"masih ada bukti yg menunggu verifikasi untuk order ini")
	case errors.Is(err, paymentapi.ErrProofAlreadyReviewed):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"bukti pembayaran sudah pernah di-review")
	case errors.Is(err, paymentapi.ErrInvalidMetodeBayar):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "metode_bayar tidak valid")
	case errors.Is(err, paymentapi.ErrInvalidMimeType):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation,
			"format file tidak didukung — upload JPG/PNG/WEBP/PDF")
	case errors.Is(err, paymentapi.ErrFileTooLarge):
		httpx.Error(c, http.StatusRequestEntityTooLarge, httpx.CodeValidation,
			"ukuran file melebihi batas")
	case errors.Is(err, paymentapi.ErrFileEmpty):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "file kosong")
	case errors.Is(err, paymentapi.ErrNotOrderOwner):
		httpx.Error(c, http.StatusForbidden, httpx.CodeForbidden,
			"tidak boleh upload bukti untuk order milik orang lain")
	case errors.Is(err, paymentapi.ErrRejectReasonRequired):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "reason wajib diisi")
	case errors.Is(err, orderapi.ErrOrderNotFound):
		httpx.Error(c, http.StatusNotFound, httpx.CodeNotFound, "order tidak ditemukan")
	case errors.Is(err, orderapi.ErrPaymentAlreadySettled):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict, "order sudah lunas/selesai/dibatalkan")
	case errors.Is(err, orderapi.ErrOrderStateChanged):
		httpx.Error(c, http.StatusConflict, httpx.CodeConflict,
			"status order berubah bersamaan, refresh dan coba lagi")
	default:
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("payment service error")
		httpx.Error(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
