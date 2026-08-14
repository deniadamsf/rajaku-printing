package service

import (
	"fmt"

	"github.com/rajaku-printing/backend/internal/notification/notificationapi"
)

// templateCtx — data yang dipakai template. Field opsional per-kind diisi
// lewat `extras` map di call site (mis. shipping_cost, reason).
type templateCtx struct {
	CustomerName string
	Resi         string
	BaseURL      string // trackingURL = BaseURL + /lacak/:resi
	Total        int64  // idr rupiah, tanpa desimal
	Extras       map[string]any
}

// trackingURL — dibaca dari SATU sumber (config.App.BaseURL) sesuai §2
// "konfigurasi URL terpusat".
func (c templateCtx) trackingURL() string {
	return fmt.Sprintf("%s/lacak/%s", c.BaseURL, c.Resi)
}

// render mengembalikan pesan WA untuk `kind`, atau ErrUnknownKind kalau kind
// belum di-implementasi. Pure — no I/O, gampang di-test.
//
// Trigger yang di-support saat ini (spec §13, MVP):
//   - ongkir_ready          — dari order.SetShippingCost / ConfirmPickupTotal
//   - payment_verified      — dari payment.ApproveProof
//   - payment_rejected      — dari payment.RejectProof
//   - design_approved       — dari design.StaffVerifyUpload (upload path) atau
//                              design.ApproveDraft (request path — konfirmasi ke customer)
//   - design_needs_revision — dari design.RequestRevision (konfirmasi ke customer
//                              bahwa revisi diteruskan ke desainer)
//   - ready_pickup          — dari production.MarkSiapKirimAtauAmbil (pickup path)
//   - ready_ship            — dari production.MarkSiapKirimAtauAmbil (kirim path)
//   - shipped               — dari production.MarkDikirim (dgn courier + tracking)
//   - invoice_ready         — dari invoice.GenerateForOrder (dgn download URL)
//   - pos_order_created     — dari pos.CreateOrder (walk-in confirmation)
//
// Kind lain di notificationapi (KindInvoiceReady dll) sengaja belum punya
// template — akan ditambah saat modul consumer-nya dibangun (invoice, POS).
// Return ErrUnknownKind, bukan template dummy, biar consumer yang salah
// nge-enqueue kind langsung ketauan (§22 no-silent-stub).
func render(kind notificationapi.Kind, ctx templateCtx) (string, error) {
	switch kind {
	case notificationapi.KindOngkirReady:
		return renderOngkirReady(ctx), nil
	case notificationapi.KindPaymentVerified:
		return renderPaymentVerified(ctx), nil
	case notificationapi.KindPaymentRejected:
		return renderPaymentRejected(ctx), nil
	case notificationapi.KindDesignApproved:
		return renderDesignApproved(ctx), nil
	case notificationapi.KindDesignNeedsRevision:
		return renderDesignNeedsRevision(ctx), nil
	case notificationapi.KindReadyPickup:
		return renderReadyPickup(ctx), nil
	case notificationapi.KindReadyShip:
		return renderReadyShip(ctx), nil
	case notificationapi.KindShipped:
		return renderShipped(ctx), nil
	case notificationapi.KindInvoiceReady:
		return renderInvoiceReady(ctx), nil
	case notificationapi.KindPOSOrderCreated:
		return renderPOSOrderCreated(ctx), nil
	case notificationapi.KindDesignRetentionWarning:
		return renderDesignRetentionWarning(ctx), nil
	default:
		return "", notificationapi.ErrUnknownKind
	}
}

func renderOngkirReady(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s 👋\n\nPesanan Anda dengan nomor resi *%s* sudah dikonfirmasi.\n"+
			"Total pembayaran: *Rp %s*\n\n"+
			"Silakan lakukan transfer/QRIS, lalu upload bukti bayar di:\n%s\n\n"+
			"Terima kasih — Rajaku Printing.",
		c.CustomerName, c.Resi, formatIDR(c.Total), c.trackingURL(),
	)
}

func renderPaymentVerified(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s ✅\n\nPembayaran untuk pesanan *%s* sudah kami verifikasi.\n"+
			"Pesanan akan segera masuk proses produksi.\n\n"+
			"Pantau status di:\n%s",
		c.CustomerName, c.Resi, c.trackingURL(),
	)
}

func renderPaymentRejected(c templateCtx) string {
	reason, _ := c.Extras["reason"].(string)
	if reason == "" {
		reason = "bukti tidak jelas / tidak sesuai"
	}
	return fmt.Sprintf(
		"Halo %s ⚠️\n\nBukti pembayaran untuk pesanan *%s* belum bisa kami terima.\n"+
			"Alasan: %s\n\n"+
			"Mohon upload ulang bukti transfer/QRIS di:\n%s",
		c.CustomerName, c.Resi, reason, c.trackingURL(),
	)
}

func renderDesignApproved(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s ✅\n\nDesain untuk pesanan *%s* sudah disetujui.\n"+
			"Kami lanjutkan ke proses cetak. Terima kasih.\n\n"+
			"Pantau status di:\n%s",
		c.CustomerName, c.Resi, c.trackingURL(),
	)
}

func renderDesignNeedsRevision(c templateCtx) string {
	note, _ := c.Extras["note"].(string)
	body := "Desainer akan segera mengerjakan revisi."
	if note != "" {
		body = fmt.Sprintf("Catatan revisi: %s\nDesainer akan segera mengerjakan revisi.", note)
	}
	return fmt.Sprintf(
		"Halo %s ✍️\n\nPermintaan revisi untuk pesanan *%s* sudah kami terima.\n%s\n\n"+
			"Kami hubungi lagi saat draft baru siap.\n\n%s",
		c.CustomerName, c.Resi, body, c.trackingURL(),
	)
}

func renderReadyPickup(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s 📦\n\nPesanan *%s* sudah selesai dan siap diambil di toko.\n"+
			"Silakan datang dengan menunjukkan nomor resi ini.\n\n"+
			"Detail:\n%s\n\nRajaku Printing.",
		c.CustomerName, c.Resi, c.trackingURL(),
	)
}

func renderReadyShip(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s 📦\n\nPesanan *%s* sudah selesai dan akan segera dikirim.\n"+
			"Nomor resi ekspedisi akan kami kirim setelah paket diserahkan ke kurir.\n\n"+
			"Detail:\n%s",
		c.CustomerName, c.Resi, c.trackingURL(),
	)
}

func renderPOSOrderCreated(c templateCtx) string {
	return fmt.Sprintf(
		"Halo %s 👋\n\nTerima kasih sudah order di Rajaku Printing.\n"+
			"Nomor resi Anda: *%s*\n"+
			"Total dibayar: *Rp %s*\n\n"+
			"Lacak progres di:\n%s",
		c.CustomerName, c.Resi, formatIDR(c.Total), c.trackingURL(),
	)
}

func renderInvoiceReady(c templateCtx) string {
	invoiceURL, _ := c.Extras["invoice_url"].(string)
	invoiceNumber, _ := c.Extras["invoice_number"].(string)
	numStr := ""
	if invoiceNumber != "" {
		numStr = " (" + invoiceNumber + ")"
	}
	if invoiceURL == "" {
		invoiceURL = c.trackingURL()
	}
	return fmt.Sprintf(
		"Halo %s 🧾\n\nInvoice%s untuk pesanan *%s* sudah siap.\n"+
			"Download di:\n%s\n\nSimpan sebagai bukti transaksi. Terima kasih.",
		c.CustomerName, numStr, c.Resi, invoiceURL,
	)
}

func renderShipped(c templateCtx) string {
	courier, _ := c.Extras["courier"].(string)
	tracking, _ := c.Extras["tracking"].(string)
	if courier == "" {
		courier = "(kurir tidak diinput)"
	}
	if tracking == "" {
		tracking = "(no. resi tidak diinput)"
	}
	return fmt.Sprintf(
		"Halo %s 🚚\n\nPesanan *%s* sudah dikirim.\n"+
			"Kurir: *%s*\nNo. Resi Ekspedisi: *%s*\n\n"+
			"Lacak: %s",
		c.CustomerName, c.Resi, courier, tracking, c.trackingURL(),
	)
}

// renderDesignRetentionWarning — INTERNAL, dikirim ke nomor ops (§19).
// Sengaja beda gaya dari template customer: diawali penanda [INTERNAL] supaya
// kalau nomor ops kebetulan sama dengan nomor toko, staff langsung tahu ini
// pesan sistem, bukan pesan yang harus diteruskan ke pelanggan.
func renderDesignRetentionWarning(c templateCtx) string {
	daysLeft := extraInt(c.Extras, "days_left", 3)
	retentionDays := extraInt(c.Extras, "retention_days", 30)
	fileName, _ := c.Extras["file_name"].(string)
	orderStatus, _ := c.Extras["order_status"].(string)
	if fileName == "" {
		fileName = "(nama file tidak tercatat)"
	}
	if orderStatus == "" {
		orderStatus = "(status tidak diketahui)"
	}
	return fmt.Sprintf(
		"[INTERNAL] ⚠️ Retensi file desain\n\n"+
			"File *%s* pada order *%s* akan dihapus otomatis dari server dalam *%d hari* "+
			"(kebijakan retensi %d hari).\n"+
			"Status order saat ini: *%s* — belum selesai, jadi kemungkinan masih perlu reprint.\n\n"+
			"Download dulu kalau masih dibutuhkan:\n%s/admin/desain/%s\n\n"+
			"Setelah dihapus, catatan file tetap ada di sistem tapi filenya tidak bisa diunduh lagi.",
		fileName, c.Resi, daysLeft, retentionDays, orderStatus, c.BaseURL, c.Resi,
	)
}

// extraInt membaca angka dari extras map. Nilai bisa datang sebagai int
// (call site Go langsung) atau float64 (kalau pernah lewat JSON round-trip),
// jadi keduanya ditangani; selain itu pakai fallback.
func extraInt(extras map[string]any, key string, fallback int) int {
	switch v := extras[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

// formatIDR — separator ribuan pakai titik (konvensi id-ID).
// Input dalam satuan rupiah (int64).
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
