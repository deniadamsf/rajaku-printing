// Package workerclient is a small HTTP client for the notification-worker
// (Node.js/Baileys microservice, §13) WhatsApp pairing endpoints:
//
//	GET  {WorkerURL}/pairing         — status pairing/QR
//	POST {WorkerURL}/pairing/logout  — putus pairing
//
// Both endpoints require the shared X-Internal-Secret header (same secret
// the worker itself uses when calling BACK into this backend's
// /internal/notifications/* — here the direction is reversed, we are the
// caller). Internal to the notification module; not exported cross-module.
package workerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sentinel errors — the notification/service layer branches behavior on
// these via errors.Is (§22/§23: sentinel error terpisah untuk error yang
// perlu dibedakan penanganannya).
var (
	// ErrWorkerUnreachable — worker tidak bisa dihubungi sama sekali: koneksi
	// gagal/timeout, atau balasan dengan status HTTP yang tidak dikenal
	// selain 401. Ini kondisi yang dimaksud "worker mati" oleh caller.
	ErrWorkerUnreachable = errors.New("workerclient: notification worker tidak bisa dihubungi")
	// ErrUnauthorized — worker menjawab 401 (X-Internal-Secret salah/hilang).
	// Sengaja DIBEDAKAN dari ErrWorkerUnreachable: worker hidup dan
	// menjawab, hanya kredensial internal yang salah (bug konfigurasi kita,
	// bukan worker yang mati) — operator butuh pesan yang berbeda.
	ErrUnauthorized = errors.New("workerclient: secret internal ditolak notification worker (401)")
)

// defaultTimeout — batas waktu total per request kalau caller tidak
// menentukan timeout sendiri. Sengaja pendek: halaman admin tidak boleh
// menggantung kalau worker mati/lambat.
const defaultTimeout = 5 * time.Second

const secretHeader = "X-Internal-Secret"

// PairingStatus — bentuk JSON balasan GET /pairing. Quota/Circuit/Pacing
// sengaja json.RawMessage: bentuknya ditentukan bebas oleh notification-worker
// dan diteruskan apa adanya ke frontend, bukan dipetakan ke struct kaku.
type PairingStatus struct {
	WAReady    bool            `json:"wa_ready"`
	SelfNumber *string         `json:"self_number"`
	QRDataURL  *string         `json:"qr_data_url"`
	Quota      json.RawMessage `json:"quota"`
	Circuit    json.RawMessage `json:"circuit"`
	Pacing     json.RawMessage `json:"pacing"`
}

// Client talks to the notification-worker pairing endpoints. Zero value is
// not usable — construct via New.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// New constructs a Client. baseURL and secret normally come from
// config.Config.Notification (WorkerURL/InternalSecret), read once at
// startup (§2/§22 — no scattered os.Getenv). timeout <= 0 falls back to
// defaultTimeout.
func New(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		http:    &http.Client{Timeout: timeout},
	}
}

// PairingStatus fetches the current pairing/QR state from the worker.
func (c *Client) PairingStatus(ctx context.Context) (*PairingStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/pairing", nil)
	if err != nil {
		return nil, fmt.Errorf("bangun request GET /pairing: %w", err)
	}
	req.Header.Set(secretHeader, c.secret)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panggil worker GET /pairing: %w", errors.Join(ErrWorkerUnreachable, err))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("baca response GET /pairing: %w", errors.Join(ErrWorkerUnreachable, err))
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// lanjut decode di bawah.
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("worker GET /pairing: %w", ErrUnauthorized)
	default:
		return nil, fmt.Errorf("worker GET /pairing balas status tidak terduga %d: %w", resp.StatusCode, ErrWorkerUnreachable)
	}

	var st PairingStatus
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("decode response GET /pairing: %w", errors.Join(ErrWorkerUnreachable, err))
	}
	return &st, nil
}

// Logout memutus pairing worker (QR baru bisa diterbitkan setelahnya).
func (c *Client) Logout(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/pairing/logout", nil)
	if err != nil {
		return fmt.Errorf("bangun request POST /pairing/logout: %w", err)
	}
	req.Header.Set(secretHeader, c.secret)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("panggil worker POST /pairing/logout: %w", errors.Join(ErrWorkerUnreachable, err))
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("worker POST /pairing/logout: %w", ErrUnauthorized)
	default:
		return fmt.Errorf("worker POST /pairing/logout balas status tidak terduga %d: %w", resp.StatusCode, ErrWorkerUnreachable)
	}
}
