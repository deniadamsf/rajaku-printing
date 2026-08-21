// Package state defines the order lifecycle state machine (spec section 4).
//
// Semua status disimpan sebagai string di DB. Package ini exposes:
//   - Konstanta status (satu sumber, cegah typo tersebar).
//   - Transisi valid (map from → allowed set).
//   - Helper: IsTerminal, IsValidTransition, IsKnown.
//
// Modul order (dan modul lain yg advance status) WAJIB pakai IsValidTransition
// sebelum update DB — jangan asumsikan transisi manapun boleh.
package state

// Status codes — satu sumber. Nama sengaja sama persis dgn spec section 4.
type Status string

const (
	OrderMasuk             Status = "order_masuk"
	MenungguOngkir         Status = "menunggu_ongkir"
	MenungguPembayaran     Status = "menunggu_pembayaran"
	MenungguVerifikasi     Status = "menunggu_verifikasi"
	Dibayar                Status = "dibayar"
	Ditolak                Status = "ditolak"
	DesainDiverifikasi     Status = "desain_diverifikasi"
	DesainDikerjakan       Status = "desain_dikerjakan"
	MenungguApprovalDesain Status = "menunggu_approval_desain"
	ProsesCetak            Status = "proses_cetak"
	QC                     Status = "qc"
	SiapKirim              Status = "siap_kirim"
	SiapAmbil              Status = "siap_ambil"
	Dikirim                Status = "dikirim"
	Selesai                Status = "selesai"
	Dibatalkan             Status = "dibatalkan"
)

// All returns semua status yg dikenal — dipakai testing & validation.
func All() []Status {
	return []Status{
		OrderMasuk, MenungguOngkir, MenungguPembayaran, MenungguVerifikasi,
		Dibayar, Ditolak,
		DesainDiverifikasi, DesainDikerjakan, MenungguApprovalDesain,
		ProsesCetak, QC,
		SiapKirim, SiapAmbil, Dikirim,
		Selesai, Dibatalkan,
	}
}

// IsKnown returns true kalau s adalah salah satu status yg dikenal.
func IsKnown(s Status) bool {
	for _, k := range All() {
		if k == s {
			return true
		}
	}
	return false
}

// IsTerminal returns true untuk status akhir yg tidak boleh diadvance lagi.
func IsTerminal(s Status) bool {
	return s == Selesai || s == Dibatalkan
}

// transitions — from → set of allowed next status.
// Dibatalkan bisa dari status manapun sebelum proses_cetak — di-handle di IsValidTransition.
var transitions = map[Status]map[Status]struct{}{
	OrderMasuk: {
		MenungguOngkir:     {}, // kirim (admin isi ongkir)
		MenungguPembayaran: {}, // pickup (total sudah fix, skip ongkir)
	},
	MenungguOngkir: {
		MenungguPembayaran: {},
	},
	MenungguPembayaran: {
		MenungguVerifikasi: {},
		Dibayar:            {}, // POS cash/qris di tempat (section 11)
	},
	MenungguVerifikasi: {
		Dibayar: {},
		Ditolak: {},
	},
	Ditolak: {
		MenungguVerifikasi: {}, // customer upload ulang bukti
	},
	Dibayar: {
		DesainDiverifikasi: {}, // upload desain valid → lanjut cetak, atau butuh desain
		DesainDikerjakan:   {}, // request desain
	},
	DesainDikerjakan: {
		MenungguApprovalDesain: {}, // staff finish draft → tunggu customer
		DesainDiverifikasi:     {}, // walk-in POS "Disetujui Langsung" (§11)
	},
	MenungguApprovalDesain: {
		DesainDikerjakan:   {}, // revisi
		DesainDiverifikasi: {}, // customer approve
	},
	// Note: DesainDikerjakan → DesainDiverifikasi juga valid untuk walk-in POS
	// (§11 "Disetujui Langsung" — skip MenungguApprovalDesain). Ditambah di sini
	// biar state machine yg jadi authority tunggal transisi valid; service
	// guard-nya berdasarkan order.design_approval_mode + channel.
	DesainDiverifikasi: {
		ProsesCetak: {},
	},
	ProsesCetak: {
		QC: {},
	},
	QC: {
		SiapKirim: {}, // metode_ambil=kirim
		SiapAmbil: {}, // metode_ambil=pickup
	},
	SiapKirim: {
		Dikirim: {},
	},
	Dikirim: {
		Selesai: {},
	},
	SiapAmbil: {
		Selesai: {},
	},
	// Terminal — no transitions
	Selesai:    {},
	Dibatalkan: {},
}

// preCetakStates — status yg boleh langsung di-cancel (section 4:
// "dibatalkan bisa terjadi dari status manapun sebelum proses_cetak, mis. timeout bayar").
var preCetakStates = map[Status]struct{}{
	OrderMasuk:             {},
	MenungguOngkir:         {},
	MenungguPembayaran:     {},
	MenungguVerifikasi:     {},
	Ditolak:                {},
	Dibayar:                {},
	DesainDikerjakan:       {},
	MenungguApprovalDesain: {},
	DesainDiverifikasi:     {},
}

// IsValidTransition returns true iff from → to adalah transisi yg diizinkan.
// Cancel path (→ Dibatalkan) diperlakukan khusus: boleh dari status apapun
// sebelum ProsesCetak.
func IsValidTransition(from, to Status) bool {
	if !IsKnown(from) || !IsKnown(to) {
		return false
	}
	if to == Dibatalkan {
		_, ok := preCetakStates[from]
		return ok
	}
	allowed, ok := transitions[from]
	if !ok {
		return false
	}
	_, permitted := allowed[to]
	return permitted
}

// NextStates returns semua status yg valid dari `from` (termasuk Dibatalkan
// kalau `from` masih pre-cetak). Untuk UI dropdown / validasi client-side.
func NextStates(from Status) []Status {
	out := []Status{}
	for s := range transitions[from] {
		out = append(out, s)
	}
	if _, canCancel := preCetakStates[from]; canCancel {
		out = append(out, Dibatalkan)
	}
	return out
}

// preDibayarStatuses — statuses BEFORE the order has been paid (§4). Dipakai
// (a) modul order/service untuk menentukan field finansial mana yg boleh
// diedit bebas oleh super admin tanpa reason panjang (§ super admin order
// tools), dan (b) IsDeletable di bawah.
var preDibayarStatuses = map[Status]struct{}{
	OrderMasuk:         {},
	MenungguOngkir:     {},
	MenungguPembayaran: {},
	MenungguVerifikasi: {},
	Ditolak:            {},
}

// IsPreDibayar returns true kalau s adalah status SEBELUM order dibayar.
func IsPreDibayar(s Status) bool {
	_, ok := preDibayarStatuses[s]
	return ok
}

// IsDeletable returns true kalau order berstatus s boleh di-soft-delete oleh
// super admin (§ super admin order tools). Order yang SUDAH dibayar (atau
// lanjut ke status manapun sesudahnya) TIDAK boleh langsung dihapus — uangnya
// sudah di tangan toko, menghapus order itu akan membuatnya lenyap dari
// rekap kas (ListPOSByDateRange dkk. memfilter deleted_at IS NULL) tanpa
// jejak. Satu-satunya jalan keluar untuk order berbayar adalah dibatalkan
// dulu (lewat cancel/OverrideStatus ke Dibatalkan) — status Dibatalkan itu
// sendiri TETAP boleh dihapus (order gagal/dibatalkan tidak menyumbang kas).
func IsDeletable(s Status) bool {
	return IsPreDibayar(s) || s == Dibatalkan
}
