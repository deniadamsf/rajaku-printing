// Package pdfrender — pure function OrderInvoiceView + meta → PDF bytes.
// Sengaja terpisah dari service supaya:
//   - Bisa di-swap kalau nanti pindah ke maroto atau engine lain.
//   - Bisa di-test tanpa DB / notifier / filestore.
//   - Layout & branding tersentralisasi di satu file.
package pdfrender

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// CompanyInfo — data statis toko yg tampil di header invoice.
type CompanyInfo struct {
	Name    string
	Address string
	Phone   string
	Email   string
	Website string // biasanya = APP_BASE_URL
}

// Meta — data invoice spesifik (bukan bagian order).
type Meta struct {
	InvoiceNumber string
	GeneratedAt   time.Time
}

// Render mengembalikan bytes PDF (A4, 1 halaman untuk MVP — 1 line item).
// Tidak I/O — pure function, gampang di-test.
func Render(company CompanyInfo, meta Meta, order *orderapi.OrderInvoiceView, customerName, customerPhone string) ([]byte, error) {
	if order == nil {
		return nil, fmt.Errorf("pdfrender: order view nil")
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// --- Header ---
	pdf.SetFont("Helvetica", "B", 20)
	pdf.CellFormat(0, 10, company.Name, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	if company.Address != "" {
		pdf.CellFormat(0, 4, company.Address, "", 1, "L", false, 0, "")
	}
	contactLine := ""
	if company.Phone != "" {
		contactLine += "Telp: " + company.Phone
	}
	if company.Email != "" {
		if contactLine != "" {
			contactLine += "  |  "
		}
		contactLine += company.Email
	}
	if company.Website != "" {
		if contactLine != "" {
			contactLine += "  |  "
		}
		contactLine += company.Website
	}
	if contactLine != "" {
		pdf.CellFormat(0, 4, contactLine, "", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
	drawHorizontalRule(pdf)
	pdf.Ln(4)

	// --- Invoice title + meta (right-aligned) ---
	yStart := pdf.GetY()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 8, "INVOICE", "", 1, "R", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 5, "No. Invoice : "+meta.InvoiceNumber, "", 1, "R", false, 0, "")
	pdf.CellFormat(0, 5, "Tanggal     : "+meta.GeneratedAt.Format("02 Jan 2006"), "", 1, "R", false, 0, "")
	pdf.CellFormat(0, 5, "No. Resi    : "+order.Resi, "", 1, "R", false, 0, "")
	// Reset yStart untuk billed-to (di kolom kiri).
	yEnd := pdf.GetY()

	// --- Billed to (left column) ---
	pdf.SetY(yStart)
	pdf.SetX(15)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(80, 5, "Ditagihkan kepada:", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetX(15)
	pdf.CellFormat(80, 5, customerName, "", 1, "L", false, 0, "")
	if customerPhone != "" {
		pdf.SetX(15)
		pdf.CellFormat(80, 5, customerPhone, "", 1, "L", false, 0, "")
	}
	if order.MetodeAmbil == "kirim" {
		if order.ShippingRecipient != nil && *order.ShippingRecipient != "" {
			pdf.SetX(15)
			pdf.CellFormat(80, 5, "Penerima: "+*order.ShippingRecipient, "", 1, "L", false, 0, "")
		}
		if order.ShippingAddress != nil && *order.ShippingAddress != "" {
			pdf.SetX(15)
			pdf.MultiCell(80, 4, "Alamat: "+*order.ShippingAddress, "", "L", false)
		}
	}

	// Move down past the taller of the two columns.
	if pdf.GetY() < yEnd {
		pdf.SetY(yEnd)
	}
	pdf.Ln(4)
	drawHorizontalRule(pdf)
	pdf.Ln(4)

	// --- Line items table ---
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(240, 240, 240)
	pdf.CellFormat(80, 8, "Deskripsi", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 8, "Ukuran", "1", 0, "C", true, 0, "")
	pdf.CellFormat(15, 8, "Qty", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "Harga", "1", 0, "R", true, 0, "")
	pdf.CellFormat(30, 8, "Subtotal", "1", 1, "R", true, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	desc := fmt.Sprintf("%s\nBahan: %s", order.ProductName, order.MaterialName)
	// Cell w/ multi-line via MultiCell trick: pakai simple single-line MVP.
	pdf.CellFormat(80, 6, order.ProductName+" ("+order.MaterialName+")", "1", 0, "L", false, 0, "")
	sizeStr := fmt.Sprintf("%dx%d cm", order.WidthCm, order.HeightCm)
	pdf.CellFormat(25, 6, sizeStr, "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 6, fmt.Sprintf("%d", order.Quantity), "1", 0, "C", false, 0, "")
	pdf.CellFormat(30, 6, "Rp "+formatIDR(order.UnitPrice), "1", 0, "R", false, 0, "")
	pdf.CellFormat(30, 6, "Rp "+formatIDR(order.Subtotal), "1", 1, "R", false, 0, "")
	_ = desc // reserved for future multi-line renderer

	// --- Totals block (right-aligned) ---
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(120, 6, "Subtotal", "", 0, "R", false, 0, "")
	pdf.CellFormat(30, 6, "Rp "+formatIDR(order.Subtotal), "", 1, "R", false, 0, "")

	// Diskon (§28.7) — HANYA ditampilkan kalau discount_amount > 0, jangan
	// pernah cetak "Diskon Rp 0". Label sudah final (nama snapshot atau
	// "Diskon" untuk manual — dihitung sekali di order module).
	if order.DiscountAmount > 0 {
		label := order.DiscountLabel
		if label == "" {
			label = "Diskon"
		}
		pdf.CellFormat(120, 6, "Diskon ("+label+")", "", 0, "R", false, 0, "")
		pdf.CellFormat(30, 6, "-Rp "+formatIDR(order.DiscountAmount), "", 1, "R", false, 0, "")
	}

	if order.ShippingCost != nil && *order.ShippingCost > 0 {
		pdf.CellFormat(120, 6, "Ongkos Kirim", "", 0, "R", false, 0, "")
		pdf.CellFormat(30, 6, "Rp "+formatIDR(*order.ShippingCost), "", 1, "R", false, 0, "")
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(120, 8, "TOTAL", "T", 0, "R", false, 0, "")
	pdf.CellFormat(30, 8, "Rp "+formatIDR(order.Total), "T", 1, "R", false, 0, "")

	// --- Meta bawah: metode bayar, kurir, resi ekspedisi ---
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 9)
	if order.MetodeBayar != "" {
		pdf.CellFormat(0, 4, "Metode Bayar: "+prettyMetodeBayar(order.MetodeBayar), "", 1, "L", false, 0, "")
	}
	if order.ShippingCourier != nil && *order.ShippingCourier != "" {
		txt := "Kurir: " + *order.ShippingCourier
		if order.ShippingTrackingNo != nil && *order.ShippingTrackingNo != "" {
			txt += " (No. Resi Ekspedisi: " + *order.ShippingTrackingNo + ")"
		}
		pdf.CellFormat(0, 4, txt, "", 1, "L", false, 0, "")
	}
	if order.Notes != nil && *order.Notes != "" {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "I", 9)
		pdf.MultiCell(0, 4, "Catatan: "+*order.Notes, "", "L", false)
	}

	// --- Footer ---
	pdf.SetY(-25)
	drawHorizontalRule(pdf)
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 4, "Terima kasih telah memesan di "+company.Name+".", "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 4, "Invoice ini digenerate otomatis — hubungi kami jika ada pertanyaan.", "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

func drawHorizontalRule(pdf *gofpdf.Fpdf) {
	x, y := pdf.GetX(), pdf.GetY()
	pageW, _ := pdf.GetPageSize()
	lm, _, rm, _ := pdf.GetMargins()
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(lm, y, pageW-rm, y)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetXY(x, y)
}

func prettyMetodeBayar(m string) string {
	switch m {
	case "transfer":
		return "Transfer Bank"
	case "qris":
		return "QRIS"
	case "cash":
		return "Tunai"
	case "qris_pos":
		return "QRIS (POS)"
	default:
		return m
	}
}

// formatIDR — separator ribuan pakai titik. Duplikat dari notification/service
// karena package ini pure & standalone (tidak boleh import notification).
func formatIDR(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := fmt.Sprintf("%d", v)
	n := len(digits)
	out := make([]byte, 0, n+n/3)
	for i, ch := range digits {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, byte(ch))
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
