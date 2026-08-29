package repository

import (
	"testing"

	"github.com/rajaku-printing/backend/internal/order/state"
)

// TestUnpaidStatuses_ExcludesOnlyMoneyNotReceived mengunci temuan review #8:
// TotalSpend di kartu detail pelanggan hanya boleh menjumlahkan order yang
// uangnya benar-benar sudah masuk. Sebelumnya query-nya cuma mengecualikan
// `dibatalkan`, sehingga lima order mangkrak di `menunggu_pembayaran` tampil
// sebagai "Total belanja Rp 2 juta" untuk pelanggan yang belum membayar
// sepeser pun — angka yang dipakai admin menimbang approval membership
// (§30.2).
//
// Test ini sengaja menurunkan ekspektasinya dari package `state` (bukan
// menuliskan ulang daftar status sebagai literal), supaya penambahan status
// baru di §4 tidak lolos diam-diam dari pengecualian ini.
func TestUnpaidStatuses_ExcludesOnlyMoneyNotReceived(t *testing.T) {
	got := make(map[string]bool, len(unpaidStatuses()))
	for _, s := range unpaidStatuses() {
		got[s] = true
	}

	for _, s := range state.All() {
		wantExcluded := state.IsPreDibayar(s) || s == state.Dibatalkan
		if got[string(s)] != wantExcluded {
			t.Errorf("status %q: dikecualikan dari TotalSpend = %v, seharusnya %v",
				s, got[string(s)], wantExcluded)
		}
	}

	// Penjagaan eksplisit untuk kasus yang paling mahal kalau salah — status
	// yang uangnya SUDAH di tangan toko tidak boleh pernah ikut dikecualikan.
	for _, s := range []state.Status{state.Dibayar, state.ProsesCetak, state.Selesai} {
		if got[string(s)] {
			t.Errorf("status %q ikut dikecualikan dari TotalSpend — uangnya sudah masuk, harus dihitung", s)
		}
	}

	// Dan status yang jelas belum dibayar harus benar-benar terkecualikan.
	for _, s := range []state.Status{state.MenungguPembayaran, state.MenungguVerifikasi, state.Ditolak, state.Dibatalkan} {
		if !got[string(s)] {
			t.Errorf("status %q TIDAK dikecualikan dari TotalSpend — uangnya belum masuk", s)
		}
	}
}
