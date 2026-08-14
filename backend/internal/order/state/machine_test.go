package state

import "testing"

func TestIsKnown(t *testing.T) {
	if !IsKnown(OrderMasuk) {
		t.Fatal("OrderMasuk should be known")
	}
	if IsKnown("not-a-real-status") {
		t.Fatal("bogus status should NOT be known")
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(Selesai) {
		t.Fatal("Selesai must be terminal")
	}
	if !IsTerminal(Dibatalkan) {
		t.Fatal("Dibatalkan must be terminal")
	}
	if IsTerminal(ProsesCetak) {
		t.Fatal("ProsesCetak is NOT terminal")
	}
}

func TestValidTransitions_HappyPath(t *testing.T) {
	// Alur normal customer online, kirim, dgn request desain
	path := []Status{
		OrderMasuk,
		MenungguOngkir,
		MenungguPembayaran,
		MenungguVerifikasi,
		Dibayar,
		DesainDikerjakan,
		MenungguApprovalDesain,
		DesainDiverifikasi,
		ProsesCetak,
		QC,
		SiapKirim,
		Dikirim,
		Selesai,
	}
	for i := 0; i < len(path)-1; i++ {
		if !IsValidTransition(path[i], path[i+1]) {
			t.Fatalf("expected valid transition %s → %s", path[i], path[i+1])
		}
	}
}

func TestValidTransitions_PickupPath(t *testing.T) {
	path := []Status{
		OrderMasuk,
		MenungguPembayaran, // pickup skip ongkir
		Dibayar,
		DesainDiverifikasi, // upload desain, no request
		ProsesCetak,
		QC,
		SiapAmbil,
		Selesai,
	}
	for i := 0; i < len(path)-1; i++ {
		if !IsValidTransition(path[i], path[i+1]) {
			t.Fatalf("expected valid transition %s → %s", path[i], path[i+1])
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	cases := []struct{ from, to Status }{
		{OrderMasuk, ProsesCetak},           // skip banyak step
		{ProsesCetak, MenungguPembayaran},   // tidak boleh mundur
		{Selesai, ProsesCetak},              // terminal
		{Dibatalkan, ProsesCetak},           // terminal
		{QC, "unknown"},                     // unknown target
	}
	for _, tc := range cases {
		if IsValidTransition(tc.from, tc.to) {
			t.Fatalf("transition %s → %s should be INVALID", tc.from, tc.to)
		}
	}
}

func TestCancel_AllowedBeforeCetak(t *testing.T) {
	cases := []Status{
		OrderMasuk, MenungguOngkir, MenungguPembayaran, MenungguVerifikasi,
		Ditolak, Dibayar, DesainDikerjakan, MenungguApprovalDesain, DesainDiverifikasi,
	}
	for _, from := range cases {
		if !IsValidTransition(from, Dibatalkan) {
			t.Fatalf("cancel should be allowed from %s", from)
		}
	}
}

func TestCancel_ForbiddenFromCetakOnwards(t *testing.T) {
	cases := []Status{ProsesCetak, QC, SiapKirim, SiapAmbil, Dikirim, Selesai, Dibatalkan}
	for _, from := range cases {
		if IsValidTransition(from, Dibatalkan) {
			t.Fatalf("cancel should NOT be allowed from %s (spec: cancel only pre-cetak)", from)
		}
	}
}

func TestRejectedRoundTrip(t *testing.T) {
	// Ditolak → MenungguVerifikasi (customer re-upload) → Dibayar
	if !IsValidTransition(MenungguVerifikasi, Ditolak) {
		t.Fatal("verify → ditolak should be valid")
	}
	if !IsValidTransition(Ditolak, MenungguVerifikasi) {
		t.Fatal("ditolak → menunggu_verifikasi should be valid")
	}
	if !IsValidTransition(MenungguVerifikasi, Dibayar) {
		t.Fatal("verify → dibayar should be valid")
	}
}

func TestNextStates_IncludesCancelWhenAllowed(t *testing.T) {
	next := NextStates(MenungguOngkir)
	sawCancel := false
	for _, s := range next {
		if s == Dibatalkan {
			sawCancel = true
		}
	}
	if !sawCancel {
		t.Fatal("NextStates(MenungguOngkir) should include Dibatalkan")
	}
	if len(NextStates(Selesai)) != 0 {
		t.Fatalf("NextStates(Selesai) should be empty (terminal), got %v", NextStates(Selesai))
	}
}
