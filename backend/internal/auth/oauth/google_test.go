package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient builds a GoogleClient pointed at fake token/userinfo servers.
func newTestClient(t *testing.T, tokenURL, userinfoURL string) *GoogleClient {
	t.Helper()
	c := NewGoogleClient("test-client-id", "test-client-secret", "http://localhost:8080/api/v1/auth/google/callback")
	c.tokenURL = tokenURL
	c.userinfoURL = userinfoURL
	return c
}

func TestGoogleClient_Exchange_HappyPath(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("code") != "good-code" {
			t.Fatalf("unexpected code: %q", r.FormValue("code"))
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Fatalf("unexpected grant_type: %q", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-access-token" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GoogleProfile{
			Subject:       "1234567890",
			Email:         "budi@example.com",
			EmailVerified: true,
			Name:          "Budi Santoso",
		})
	}))
	defer userinfoSrv.Close()

	c := newTestClient(t, tokenSrv.URL, userinfoSrv.URL)
	profile, err := c.Exchange(context.Background(), "good-code")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if profile.Subject != "1234567890" {
		t.Errorf("expected subject 1234567890, got %q", profile.Subject)
	}
	if profile.Email != "budi@example.com" {
		t.Errorf("expected email budi@example.com, got %q", profile.Email)
	}
	if !profile.EmailVerified {
		t.Error("expected email_verified=true")
	}
}

func TestGoogleClient_Exchange_TokenEndpointRejectsCode(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "Malformed auth code.",
		})
	}))
	defer tokenSrv.Close()

	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("userinfo endpoint should not be called when token exchange fails")
	}))
	defer userinfoSrv.Close()

	c := newTestClient(t, tokenSrv.URL, userinfoSrv.URL)
	_, err := c.Exchange(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("expected error to mention invalid_grant, got: %v", err)
	}
}

func TestGoogleClient_AuthCodeURL_ContainsExpectedParams(t *testing.T) {
	c := NewGoogleClient("cid", "secret", "http://localhost:8080/callback")
	u := c.AuthCodeURL("nonce-123")
	for _, want := range []string{
		"client_id=cid",
		"state=nonce-123",
		"response_type=code",
		"scope=openid+email+profile",
		"access_type=online",
		"prompt=select_account",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("AuthCodeURL() = %q, expected to contain %q", u, want)
		}
	}
}
