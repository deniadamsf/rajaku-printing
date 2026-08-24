package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// makeProductIDsRaw builds the map[string]json.RawMessage parseUpdateInput
// expects, with a product_ids array of the given length.
func makeProductIDsRaw(t *testing.T, n int) map[string]json.RawMessage {
	t.Helper()
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	b, err := json.Marshal(ids)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return map[string]json.RawMessage{"product_ids": json.RawMessage(b)}
}

// TestParseUpdateInput_ProductIDsTooMany_Rejected — temuan review #6: tanpa
// batas, product_ids yang sangat panjang berakhir sebagai satu INSERT
// dengan 2 parameter/baris yang bisa melampaui batas 65.535 parameter
// protokol Postgres, meledak 500 alih-alih 400 yang jelas.
func TestParseUpdateInput_ProductIDsTooMany_Rejected(t *testing.T) {
	_, err := parseUpdateInput(makeProductIDsRaw(t, 501))
	if err == nil {
		t.Fatalf("parseUpdateInput() error = nil, want error (501 product_ids exceeds limit)")
	}
}

// TestParseUpdateInput_ProductIDsAtLimit_Accepted — boundary: tepat 500
// masih diterima (limitnya "lebih dari 500", bukan "500 ke atas").
func TestParseUpdateInput_ProductIDsAtLimit_Accepted(t *testing.T) {
	in, err := parseUpdateInput(makeProductIDsRaw(t, 500))
	if err != nil {
		t.Fatalf("parseUpdateInput() error = %v, want nil", err)
	}
	if in.ProductIDs == nil || len(*in.ProductIDs) != 500 {
		t.Fatalf("parseUpdateInput() ProductIDs = %+v, want 500 items", in.ProductIDs)
	}
}

// TestParseUpdateInput_AppliesToEmptyString_PassesThroughAsSent — dto layer
// hanya bertugas membedakan "tidak dikirim" (nil) dari "dikirim" (non-nil),
// TERMASUK kalau isinya string kosong; penolakan "" itu sendiri adalah
// tanggung jawab service (temuan review #2 — lihat
// discount_service_test.go TestUpdate_AppliesToEmptyString_Rejected).
func TestParseUpdateInput_AppliesToEmptyString_PassesThroughAsSent(t *testing.T) {
	in, err := parseUpdateInput(map[string]json.RawMessage{"applies_to": json.RawMessage(`""`)})
	if err != nil {
		t.Fatalf("parseUpdateInput() error = %v, want nil", err)
	}
	if in.AppliesTo == nil || *in.AppliesTo != "" {
		t.Fatalf("parseUpdateInput() AppliesTo = %v, want pointer to empty string (explicitly sent)", in.AppliesTo)
	}
}
