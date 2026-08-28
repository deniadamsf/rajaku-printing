package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestCreate_ProductIDsTooMany_Returns400 — temuan review #6, boundary di
// jalur HTTP (binding tag di createDiscountRequest.ProductIDs). Karena
// binding gagal SEBELUM handler pernah menyentuh service, svc boleh nil.
func TestCreate_ProductIDsTooMany_Returns400(t *testing.T) {
	h := New(nil)
	ids := make([]uuid.UUID, 501)
	for i := range ids {
		ids[i] = uuid.New()
	}
	body, err := json.Marshal(map[string]any{
		"code": "PROMO1", "name": "Promo", "type": "percent", "value_percent": 10,
		"applies_to": "selected", "product_ids": ids,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	c, rec := newTestContext(http.MethodPost, "/admin/discounts", body)
	h.Create(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Create() status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestApplicable_ExplicitNilProductID_Returns400 — temuan review #4 (efek
// samping): product_id=00000000-0000-0000-0000-000000000000 dikirim
// eksplisit harus ditolak, bukan diperlakukan sama dengan "tidak dikirim".
func TestApplicable_ExplicitNilProductID_Returns400(t *testing.T) {
	h := New(nil)
	c, rec := newTestContext(http.MethodGet,
		"/admin/discounts/applicable?channel=pos&subtotal=1000&product_id="+uuid.Nil.String(), nil)
	h.Applicable(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Applicable() status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestApplicable_ExplicitNilCustomerID_Returns400 — §30.3, mirror
// TestApplicable_ExplicitNilProductID_Returns400: customer_id=00000000-...
// dikirim eksplisit harus ditolak, bukan diperlakukan sama dengan "tidak
// dikirim" (yang juga memakai uuid.Nil sebagai sentinel "tidak difilter").
func TestApplicable_ExplicitNilCustomerID_Returns400(t *testing.T) {
	h := New(nil)
	c, rec := newTestContext(http.MethodGet,
		"/admin/discounts/applicable?channel=pos&subtotal=1000&customer_id="+uuid.Nil.String(), nil)
	h.Applicable(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Applicable() status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestApplicable_InvalidCustomerID_Returns400 — customer_id yang bukan UUID
// sama sekali juga ditolak 400, bukan diam-diam diabaikan.
func TestApplicable_InvalidCustomerID_Returns400(t *testing.T) {
	h := New(nil)
	c, rec := newTestContext(http.MethodGet,
		"/admin/discounts/applicable?channel=pos&subtotal=1000&customer_id=bukan-uuid", nil)
	h.Applicable(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Applicable() status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestCreate_CustomerIDsTooMany_Returns400 — §30.3 mirror
// TestCreate_ProductIDsTooMany_Returns400.
func TestCreate_CustomerIDsTooMany_Returns400(t *testing.T) {
	h := New(nil)
	ids := make([]uuid.UUID, 501)
	for i := range ids {
		ids[i] = uuid.New()
	}
	body, err := json.Marshal(map[string]any{
		"code": "PROMO1", "name": "Promo", "type": "percent", "value_percent": 10,
		"audience_scope": "member", "member_scope": "selected_members", "customer_ids": ids,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	c, rec := newTestContext(http.MethodPost, "/admin/discounts", body)
	h.Create(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Create() status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

func newTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	c.Request = req
	return c, rec
}
