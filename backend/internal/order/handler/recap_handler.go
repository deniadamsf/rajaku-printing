package handler

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/httpx"
	"github.com/rajaku-printing/backend/internal/order/orderapi"
	"github.com/rajaku-printing/backend/internal/order/service"
	"github.com/rajaku-printing/backend/internal/pkg/csvsafe"
)

// GET /admin/order-recap — laporan rekap order (§28.5).
// Requires: RequireUserType(staff) + RequirePermission("report.view").
func (h *Handler) AdminOrderRecap(c *gin.Context) {
	f, err := parseRecapQuery(c)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}
	res, err := h.svc.OrderRecap(c.Request.Context(), *f)
	if err != nil {
		h.mapRecapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// recapCSVFlushEvery — flush the CSV writer to the client every N rows
// (temuan review #4b: "di-Flush berkala") instead of buffering the entire
// response, so a large export streams out progressively rather than
// building up in an internal buffer before the first byte reaches the
// client.
const recapCSVFlushEvery = 200

// recapCSVHeader — kolom sama dengan tabel admin (§28.5): angka rupiah polos
// tanpa pemisah ribuan (langsung terbaca sebagai angka di spreadsheet).
var recapCSVHeader = []string{
	"Tanggal", "Resi", "Pelanggan", "Produk", "Jumlah Item", "Channel", "Status",
	"Subtotal", "Diskon", "Nominal Diskon", "Ongkir", "Total", "Metode Bayar",
}

func recapRowToCSVRecord(r service.RecapRow) []string {
	return []string{
		r.CreatedAt,
		r.Resi,
		// csvsafe.Field — nama pelanggan sepenuhnya diisi pihak luar (sama
		// celah dengan customer_csv.go, temuan review #6): dinetralkan dari
		// formula injection sebelum ditulis sebagai sel CSV.
		csvsafe.Field(r.CustomerName),
		r.ProductName,
		strconv.FormatInt(r.ItemCount, 10),
		r.Channel,
		r.Status,
		strconv.FormatInt(r.Subtotal, 10),
		r.DiscountLabel,
		strconv.FormatInt(r.DiscountAmount, 10),
		strconv.FormatInt(r.ShippingCost, 10),
		strconv.FormatInt(r.Total, 10),
		r.MetodeBayar,
	}
}

// GET /admin/order-recap/export — ekspor CSV, filter yang sama (§28.5).
// Requires: RequireUserType(staff) + RequirePermission("report.view").
//
// Temuan review #4b — SEBELUMNYA endpoint ini memuat SELURUH hasil filter ke
// satu []RecapRow, lalu me-render itu ke SATU []byte CSV penuh di memori
// (TIGA salinan penuh dari scan tabel tak terbatas untuk satu request — di
// VPS Hostinger tunggal §19, cukup untuk mematikan backend). Sekarang CSV
// ditulis LANGSUNG ke c.Writer lewat encoding/csv, di-Flush berkala
// (recapCSVFlushEvery), dengan data diambil per-batch dari
// service.OrderRecapExportStream (yang sendirinya mengambil dari repository
// per 1000 baris via RecapListBatch) — tidak ada satu titik pun yang
// menahan seluruh hasil filter di memori sekaligus.
//
// Trade-off yang disadari: karena response STREAMING, HTTP header/status
// hanya bisa "dikunci" setelah baris CSV pertama benar-benar akan ditulis.
// Kalau filter/tanggal tidak valid, itu selalu terjadi SEBELUM baris pertama
// (toRecapRepoFilter dipanggil di awal OrderRecapExportStream, sebelum
// batch manapun diambil) — jadi error tersebut MASIH bisa dijawab dengan
// envelope 400/500 yang benar. Hanya kegagalan DB di TENGAH stream (setelah
// sebagian baris terkirim) yang harus dipotong diam-diam (klien menerima
// file terpotong, bukan envelope error yang bercampur dengan CSV yang sudah
// terkirim) — ini dicatat via structured log, bukan diabaikan tanpa jejak.
func (h *Handler) AdminOrderRecapExport(c *gin.Context) {
	fromStr, toStr := c.Query("from"), c.Query("to")
	f, err := parseRecapQuery(c)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
		return
	}

	w := csv.NewWriter(c.Writer)
	headersSent := false
	rowCount := 0
	ensureHeaders := func() error {
		if headersSent {
			return nil
		}
		filename := fmt.Sprintf("rekap-order-%s-%s.csv", fromStr, toStr)
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Status(http.StatusOK)
		if err := w.Write(recapCSVHeader); err != nil {
			return fmt.Errorf("write csv header: %w", err)
		}
		headersSent = true
		return nil
	}

	streamErr := h.svc.OrderRecapExportStream(c.Request.Context(), *f, func(r service.RecapRow) error {
		if err := ensureHeaders(); err != nil {
			return err
		}
		if err := w.Write(recapRowToCSVRecord(r)); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
		rowCount++
		if rowCount%recapCSVFlushEvery == 0 {
			w.Flush()
			if err := w.Error(); err != nil {
				return fmt.Errorf("flush csv: %w", err)
			}
		}
		return nil
	})
	if streamErr != nil {
		if !headersSent {
			// Nothing written to the client yet — still safe to answer with
			// a proper JSON error envelope (e.g. filter validation errors
			// from toRecapRepoFilter, always surfaced before any batch is
			// fetched).
			h.mapRecapErr(c, streamErr)
			return
		}
		log.Ctx(c.Request.Context()).Error().Err(streamErr).
			Int("rows_sent", rowCount).
			Msg("order recap export: stream aborted mid-write, client received a truncated CSV")
		return
	}

	if err := ensureHeaders(); err != nil { // no rows matched — still emit a valid header-only CSV
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("order recap export: write empty header")
		return
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Ctx(c.Request.Context()).Error().Err(err).Msg("order recap export: final flush")
	}
}

// GET /admin/order-recap/filters?from=...&to=... — dropdown kasir/diskon
// untuk halaman rekap (fitur baru §28): mengisi filter "kasir" dan "diskon"
// dengan nilai yang BENAR-BENAR muncul di order pada rentang tanggal itu,
// bukan kotak teks UUID. Aturan tanggal & batas 366 hari SAMA seperti
// AdminOrderRecap/AdminOrderRecapExport.
// Requires: RequireUserType(staff) + RequirePermission("report.view").
func (h *Handler) AdminOrderRecapFilters(c *gin.Context) {
	fromStr, toStr := c.Query("from"), c.Query("to")
	if fromStr == "" || toStr == "" {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "query param 'from' dan 'to' wajib (format YYYY-MM-DD)")
		return
	}
	loc := jakartaLocation()
	from, err := time.ParseInLocation("2006-01-02", fromStr, loc)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "'from' invalid; format YYYY-MM-DD")
		return
	}
	to, err := time.ParseInLocation("2006-01-02", toStr, loc)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, "'to' invalid; format YYYY-MM-DD")
		return
	}

	res, err := h.svc.OrderRecapFilters(c.Request.Context(), from, to)
	if err != nil {
		h.mapRecapErr(c, err)
		return
	}
	httpx.OK(c, res)
}

// parseRecapQuery — shared query-string parsing untuk kedua endpoint rekap.
// `from`/`to` wajib (format YYYY-MM-DD, ditafsirkan zona Asia/Jakarta — pola
// sama seperti pos/handler/pos_handler.go Reconciliation).
func parseRecapQuery(c *gin.Context) (*service.RecapFilter, error) {
	fromStr, toStr := c.Query("from"), c.Query("to")
	if fromStr == "" || toStr == "" {
		return nil, fmt.Errorf("query param 'from' dan 'to' wajib (format YYYY-MM-DD)")
	}
	loc := jakartaLocation()
	from, err := time.ParseInLocation("2006-01-02", fromStr, loc)
	if err != nil {
		return nil, fmt.Errorf("'from' invalid; format YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", toStr, loc)
	if err != nil {
		return nil, fmt.Errorf("'to' invalid; format YYYY-MM-DD")
	}

	f := &service.RecapFilter{
		From:    from,
		To:      to,
		Channel: c.Query("channel"),
		Status:  c.Query("status"),
	}
	if raw := c.Query("created_by"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("created_by bukan UUID valid")
		}
		f.CreatedBy = &id
	}
	if raw := c.Query("discount_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("discount_id bukan UUID valid")
		}
		f.DiscountID = &id
	}
	if raw := c.Query("only_discounted"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("only_discounted harus boolean (true/false)")
		}
		f.OnlyDiscounted = b
	}
	// discount_manual — filter khusus diskon MANUAL, berdiri sendiri (jangan
	// dikirim bareng discount_id — dicek di service.toRecapRepoFilter,
	// dipetakan ke 400 lewat orderapi.ErrRecapDiscountFilterAmbiguous di
	// mapRecapErr). Nilai selain "true"/"false"/kosong ditolak di sini.
	if raw := c.Query("discount_manual"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("discount_manual harus boolean (true/false)")
		}
		f.DiscountManual = b
	}
	page, err := httpx.ParseIntQuery(c, "page", 0)
	if err != nil {
		return nil, err
	}
	f.Page = page
	perPage, err := httpx.ParseIntQuery(c, "per_page", 0)
	if err != nil {
		return nil, err
	}
	f.PageSize = perPage
	return f, nil
}

func jakartaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}

func (h *Handler) mapRecapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, orderapi.ErrRecapInvalidDateRange),
		errors.Is(err, orderapi.ErrRecapDateRangeTooWide),
		errors.Is(err, orderapi.ErrRecapInvalidStatus),
		errors.Is(err, orderapi.ErrRecapInvalidChannel),
		errors.Is(err, orderapi.ErrRecapDiscountFilterAmbiguous):
		httpx.Error(c, http.StatusBadRequest, httpx.CodeValidation, err.Error())
	default:
		h.mapErr(c, err)
	}
}
