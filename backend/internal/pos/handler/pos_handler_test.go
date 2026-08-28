// Package handler tests — locks the sentinel-error propagation chain
// order/service -> pos/service -> pos/handler (§30.3 code review finding
// "Fix #1"). Before the fix, pos/service wrapped every non-explicitly-listed
// error from orderapi.CreatePOSOrder with `fmt.Errorf("%w: %v", ...)` — the
// `%v` broke errors.Is, so ANY discount sentinel (old §28.9 ones AND the new
// §30.3 membership ones) fell through mapDomainErr's specific cases straight
// to the generic ErrOrderCreate 500 branch. These tests exercise
// mapDomainErr directly against an error shaped exactly like what
// pos/service.CreateOrder now produces (`%w: %w` double-wrap) — if the wrap
// ever regresses back to `%v`, errors.Is stops matching and these tests fail
// with 500 instead of the expected 4xx/422.
package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/discount/discountapi"
	"github.com/rajaku-printing/backend/internal/pos/posapi"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newErrTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/pos/orders", nil)
	return c, rec
}

// TestMapDomainErr_DiscountSentinels_PropagateThroughOrderCreateWrap mengunci
// bahwa SETIAP sentinel diskon (§28.9 lama + §30.3 baru) yang keluar dari
// order/service, dibungkus persis seperti pos/service.CreateOrder membungkus
// fallback-nya (`fmt.Errorf("%w: %w", posapi.ErrOrderCreate, err)`), tetap
// terdeteksi errors.Is oleh mapDomainErr dan dipetakan ke status HTTP yang
// benar — BUKAN jatuh ke case generic ErrOrderCreate (500).
func TestMapDomainErr_DiscountSentinels_PropagateThroughOrderCreateWrap(t *testing.T) {
	cases := []struct {
		name       string
		sentinel   error
		wantStatus int
	}{
		{"membership required (§30.3)", discountapi.ErrDiscountMembershipRequired, http.StatusUnprocessableEntity},
		{"membership disabled (§30.3)", discountapi.ErrDiscountMembershipDisabled, http.StatusUnprocessableEntity},
		{"member mismatch (§30.3)", discountapi.ErrDiscountMemberMismatch, http.StatusBadRequest},
		{"member scope empty (§30.3)", discountapi.ErrDiscountMemberScopeEmpty, http.StatusBadRequest},
		{"membership resolver unavailable (§30.3)", discountapi.ErrDiscountMembershipUnavailable, http.StatusInternalServerError},
		{"product mismatch (§28.9)", discountapi.ErrDiscountProductMismatch, http.StatusBadRequest},
		{"scope empty (§28.9)", discountapi.ErrDiscountScopeEmpty, http.StatusBadRequest},
		{"quota exhausted (§28)", discountapi.ErrDiscountQuotaExhausted, http.StatusUnprocessableEntity},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Shape identik dengan pos/service.CreateOrder's fallback wrap.
			wrapped := fmt.Errorf("%w: %w", posapi.ErrOrderCreate, tc.sentinel)

			c, rec := newErrTestContext()
			mapDomainErr(c, wrapped)

			if rec.Code != tc.wantStatus {
				t.Fatalf("mapDomainErr(%v) status = %d, want %d (body: %s)",
					tc.sentinel, rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestMapDomainErr_BrokenChain_WouldRegressTo500 dokumentasi hidup dari BUG
// yang diperbaiki: kalau wrap-nya balik pakai `%v` (bukan `%w`), errors.Is
// TIDAK bisa lagi menembus sampai ke sentinel aslinya — mapDomainErr jatuh
// ke default branch (500). Test ini TIDAK menguji kode produksi (yang sudah
// dibetulkan), melainkan menguji asumsi go-vet-level tentang %v vs %w supaya
// regresi di masa depan gampang dikenali lewat test yang gagal, bukan lewat
// laporan kasir "diskon saya ditolak tapi errornya aneh".
func TestMapDomainErr_BrokenChain_WouldRegressTo500(t *testing.T) {
	brokenWrap := fmt.Errorf("%w: %v", posapi.ErrOrderCreate, discountapi.ErrDiscountMembershipRequired)

	c, rec := newErrTestContext()
	mapDomainErr(c, brokenWrap)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("dengan %%v (broken chain) mapDomainErr status = %d, want %d — kalau ini gagal berarti "+
			"errors.Is entah bagaimana masih menembus %%v, cek ulang asumsi test ini",
			rec.Code, http.StatusInternalServerError)
	}
}
