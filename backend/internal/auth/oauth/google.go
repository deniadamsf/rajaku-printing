// Package oauth implements a minimal Google OAuth2 / OpenID Connect client
// using only the standard library (net/http + encoding/json) — deliberately
// no golang.org/x/oauth2 dependency (spec §22: don't add dependencies without
// need; the flow this project needs is a handful of HTTP calls).
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Default Google endpoints — overridable per-instance (tests point these at
// an httptest.Server).
const (
	DefaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// GoogleProfile is the subset of the OpenID Connect userinfo response the
// auth module needs.
type GoogleProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// GoogleClient talks to Google's OAuth2 authorization + token + userinfo
// endpoints. Zero value is not usable — construct via NewGoogleClient.
type GoogleClient struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client

	// authURL/tokenURL/userinfoURL are fields (not package consts baked into
	// the methods) so unit tests can redirect them at an httptest.Server.
	authURL     string
	tokenURL    string
	userinfoURL string
}

// NewGoogleClient constructs a client against the real Google endpoints.
func NewGoogleClient(clientID, clientSecret, redirectURL string) *GoogleClient {
	return &GoogleClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		authURL:      DefaultAuthURL,
		tokenURL:     DefaultTokenURL,
		userinfoURL:  DefaultUserinfoURL,
	}
}

// AuthCodeURL builds the URL the browser should be redirected to in order to
// start the consent flow. `state` is an opaque anti-CSRF nonce the caller
// must verify when the callback comes back.
func (g *GoogleClient) AuthCodeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", g.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	q.Set("state", state)
	return g.authURL + "?" + q.Encode()
}

// tokenEndpointResponse — shape of Google's POST /token JSON response
// (success and error cases share one struct — Google returns HTTP 400 with
// {"error": "...", "error_description": "..."} on failure).
type tokenEndpointResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Exchange trades an authorization `code` for the caller's profile: POST the
// code to the token endpoint, then GET the userinfo endpoint with the
// resulting bearer access token.
func (g *GoogleClient) Exchange(ctx context.Context, code string) (*GoogleProfile, error) {
	accessToken, err := g.exchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("tukar code google: %w", err)
	}
	profile, err := g.fetchProfile(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("tukar code google: %w", err)
	}
	return profile, nil
}

func (g *GoogleClient) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.clientSecret)
	form.Set("redirect_uri", g.redirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var tr tokenEndpointResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response (http %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || tr.AccessToken == "" {
		msg := tr.Error
		if tr.ErrorDesc != "" {
			msg = tr.Error + ": " + tr.ErrorDesc
		}
		if msg == "" {
			msg = fmt.Sprintf("http %d", resp.StatusCode)
		}
		return "", fmt.Errorf("token endpoint returned error: %s", msg)
	}
	return tr.AccessToken, nil
}

func (g *GoogleClient) fetchProfile(ctx context.Context, accessToken string) (*GoogleProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call userinfo endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned http %d", resp.StatusCode)
	}

	var p GoogleProfile
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode userinfo response: %w", err)
	}
	if p.Subject == "" {
		return nil, fmt.Errorf("userinfo response missing sub")
	}
	return &p, nil
}
