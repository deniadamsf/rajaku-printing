package workerclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testSecret = "unit-test-secret"

func TestPairingStatus_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pairing" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get(secretHeader); got != testSecret {
			t.Fatalf("secret header = %q, want %q", got, testSecret)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"wa_ready": true,
			"self_number": "628123456789",
			"qr_data_url": null,
			"quota": {"used": 3, "limit": 100},
			"circuit": {"state": "closed"},
			"pacing": {"delay_ms": 250}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testSecret, time.Second)
	st, err := c.PairingStatus(context.Background())
	if err != nil {
		t.Fatalf("PairingStatus() error = %v", err)
	}
	if !st.WAReady {
		t.Errorf("WAReady = false, want true")
	}
	if st.SelfNumber == nil || *st.SelfNumber != "628123456789" {
		t.Errorf("SelfNumber = %v, want 628123456789", st.SelfNumber)
	}
	if st.QRDataURL != nil {
		t.Errorf("QRDataURL = %v, want nil", st.QRDataURL)
	}
	if len(st.Quota) == 0 || len(st.Circuit) == 0 || len(st.Pacing) == 0 {
		t.Errorf("expected quota/circuit/pacing raw JSON to be passed through, got quota=%s circuit=%s pacing=%s", st.Quota, st.Circuit, st.Pacing)
	}
}

func TestPairingStatus_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "wrong-secret", time.Second)
	_, err := c.PairingStatus(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false, err = %v", err)
	}
	if errors.Is(err, ErrWorkerUnreachable) {
		t.Errorf("401 must NOT be classified as ErrWorkerUnreachable (would disguise a wrong-secret bug as \"worker down\"), err = %v", err)
	}
}

func TestPairingStatus_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close() // server sudah mati sebelum dipanggil — koneksi pasti gagal.

	c := New(deadURL, testSecret, time.Second)
	_, err := c.PairingStatus(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWorkerUnreachable) {
		t.Errorf("errors.Is(err, ErrWorkerUnreachable) = false, err = %v", err)
	}
}

func TestPairingStatus_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testSecret, 5*time.Millisecond)
	_, err := c.PairingStatus(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrWorkerUnreachable) {
		t.Errorf("errors.Is(err, ErrWorkerUnreachable) = false, err = %v", err)
	}
}

func TestLogout_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pairing/logout" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get(secretHeader); got != testSecret {
			t.Fatalf("secret header = %q, want %q", got, testSecret)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testSecret, time.Second)
	if err := c.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
}

func TestLogout_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "wrong-secret", time.Second)
	err := c.Logout(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false, err = %v", err)
	}
}

func TestLogout_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	c := New(deadURL, testSecret, time.Second)
	err := c.Logout(context.Background())
	if !errors.Is(err, ErrWorkerUnreachable) {
		t.Errorf("errors.Is(err, ErrWorkerUnreachable) = false, err = %v", err)
	}
}
