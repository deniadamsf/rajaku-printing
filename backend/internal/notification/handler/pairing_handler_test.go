package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rajaku-printing/backend/internal/notification/service"
	"github.com/rajaku-printing/backend/internal/notification/workerclient"
)

type fakePairingService struct {
	view    service.PairingStatusView
	viewErr error

	unlinkErr    error
	unlinkCalled bool
}

func (f *fakePairingService) GetPairingStatus(_ context.Context) (service.PairingStatusView, error) {
	return f.view, f.viewErr
}

func (f *fakePairingService) Unlink(_ context.Context) error {
	f.unlinkCalled = true
	return f.unlinkErr
}

func newPairingTestRouter(h *PairingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/whatsapp/pairing", h.GetPairing)
	r.POST("/admin/whatsapp/unlink", h.Unlink)
	return r
}

func strPtrH(s string) *string { return &s }

func TestPairingHandler_GetPairing_HappyPath(t *testing.T) {
	fake := &fakePairingService{
		view: service.PairingStatusView{
			Reachable:  true,
			Connected:  true,
			SelfNumber: strPtrH("628123456789"),
			QRDataURL:  nil,
			Quota:      json.RawMessage(`{"used":1}`),
			Circuit:    json.RawMessage(`{"state":"closed"}`),
			Pacing:     json.RawMessage(`{"delay_ms":100}`),
		},
	}
	r := newPairingTestRouter(NewPairingHandler(fake))

	req := httptest.NewRequest(http.MethodGet, "/admin/whatsapp/pairing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			WorkerReachable bool            `json:"worker_reachable"`
			Connected       bool            `json:"connected"`
			SelfNumber      *string         `json:"self_number"`
			QRDataURL       *string         `json:"qr_data_url"`
			Quota           json.RawMessage `json:"quota"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if !body.Success {
		t.Fatal("success = false, want true")
	}
	if !body.Data.WorkerReachable || !body.Data.Connected {
		t.Errorf("worker_reachable/connected = %v/%v, want true/true", body.Data.WorkerReachable, body.Data.Connected)
	}
	if body.Data.SelfNumber == nil || *body.Data.SelfNumber != "628123456789" {
		t.Errorf("self_number = %v", body.Data.SelfNumber)
	}
	if len(body.Data.Quota) == 0 {
		t.Error("quota should be passed through")
	}
}

func TestPairingHandler_GetPairing_WorkerUnreachable(t *testing.T) {
	fake := &fakePairingService{
		view:    service.PairingStatusView{Reachable: false},
		viewErr: workerclient.ErrWorkerUnreachable,
	}
	r := newPairingTestRouter(NewPairingHandler(fake))

	req := httptest.NewRequest(http.MethodGet, "/admin/whatsapp/pairing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Kontrak wajib: TETAP 200 walau worker tidak reachable (§ tugas).
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when worker unreachable; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			WorkerReachable bool    `json:"worker_reachable"`
			Connected       bool    `json:"connected"`
			SelfNumber      *string `json:"self_number"`
			QRDataURL       *string `json:"qr_data_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if body.Data.WorkerReachable {
		t.Error("worker_reachable = true, want false")
	}
	if body.Data.Connected {
		t.Error("connected = true, want false")
	}
	if body.Data.SelfNumber != nil || body.Data.QRDataURL != nil {
		t.Error("expected null self_number/qr_data_url when worker unreachable")
	}
}

func TestPairingHandler_Unlink_HappyPath(t *testing.T) {
	fake := &fakePairingService{}
	r := newPairingTestRouter(NewPairingHandler(fake))

	req := httptest.NewRequest(http.MethodPost, "/admin/whatsapp/unlink", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !fake.unlinkCalled {
		t.Error("expected service Unlink to be called")
	}
}

func TestPairingHandler_Unlink_WorkerUnreachable(t *testing.T) {
	fake := &fakePairingService{unlinkErr: workerclient.ErrWorkerUnreachable}
	r := newPairingTestRouter(NewPairingHandler(fake))

	req := httptest.NewRequest(http.MethodPost, "/admin/whatsapp/unlink", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Kontrak wajib: unlink saat worker mati HARUS gagal, bukan sukses palsu.
	if w.Code == http.StatusOK {
		t.Fatalf("status = 200, want failure status when worker unreachable; body=%s", w.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if body.Success {
		t.Error("success = true, want false")
	}
}
