// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"

	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

// fakeRepo is a stub tenantLookup returning a fixed User (or an error).
type fakeRepo struct {
	user *mtdb.User
	err  error
}

func (f *fakeRepo) GetUserWithTenants(_ context.Context, _ string) (*mtdb.User, error) {
	return f.user, f.err
}

// newCheckzServer builds a Server wired with a fake store + repo, bypassing
// New() (which needs Redis + OIDC discovery).
func newCheckzServer(store login.SessionStore, repo tenantLookup) *Server {
	return &Server{
		store:   store,
		cookies: login.CookieConfig{Name: "scitrera_session", Secure: false, SameSite: http.SameSiteLaxMode},
		repo:    repo,
		opts:    Options{TargetURL: "https://app.example.net"},
	}
}

func checkzReqWithSession() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/checkz", nil)
	req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
	return req
}

func sessionStore() *fakeStore {
	return &fakeStore{data: map[string]*login.SessionData{
		"sid-1": {UserID: "u@example.com", Email: "u@example.com", ExpiresAt: time.Now().Add(time.Hour)},
	}}
}

func TestCheckz_NoSession_401(t *testing.T) {
	s := newCheckzServer(&fakeStore{}, &fakeRepo{})
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, httptest.NewRequest(http.MethodGet, "/checkz", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if body := rr.Body.String(); body != `{"error":"no session"}` {
		t.Errorf("body = %q, want no-session error", body)
	}
}

func TestCheckz_ValidSession_WithTenants(t *testing.T) {
	repo := &fakeRepo{user: &mtdb.User{
		ID:              "u@example.com",
		Email:           "u@example.com",
		Enabled:         true,
		TenantSlugs:     []string{"acme", "globex"},
		TenantNames:     []string{"Acme Corp", "Globex"},
		TenantLogos:     []string{"https://logos/acme.png", ""}, // globex has no logo
		TenantDefaultWS: []string{"main"},                       // globex has no default ws (short array)
	}}
	s := newCheckzServer(sessionStore(), repo)

	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithSession())

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var got struct {
		Auth    string         `json:"auth"`
		UserID  string         `json:"user_id"`
		Email   string         `json:"email"`
		Tenants []checkzTenant `json:"tenants"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response %q: %v", rr.Body.String(), err)
	}

	if got.Auth != "valid" {
		t.Errorf("auth = %q, want valid", got.Auth)
	}
	if got.UserID != "u@example.com" || got.Email != "u@example.com" {
		t.Errorf("user_id/email = %q/%q, want u@example.com", got.UserID, got.Email)
	}
	if len(got.Tenants) != 2 {
		t.Fatalf("tenants len = %d, want 2", len(got.Tenants))
	}

	// First tenant fully populated; slug ordering preserved.
	if got.Tenants[0] != (checkzTenant{ID: "acme", Name: "Acme Corp", Logo: "https://logos/acme.png", DefaultWorkspace: "main"}) {
		t.Errorf("tenants[0] = %+v, unexpected", got.Tenants[0])
	}
	// Second tenant: empty logo (explicit "") and empty default_workspace
	// (bounds-guarded short array) both surface as empty strings.
	if got.Tenants[1] != (checkzTenant{ID: "globex", Name: "Globex", Logo: "", DefaultWorkspace: ""}) {
		t.Errorf("tenants[1] = %+v, want bounds-safe empties", got.Tenants[1])
	}

	// Parity-check.sh asserts the compact `"auth":"valid"` substring.
	if body := rr.Body.String(); !contains(body, `"auth":"valid"`) {
		t.Errorf("body %q missing compact \"auth\":\"valid\"", body)
	}
}

func TestCheckz_NilRepo_BasicResponseNoTenants(t *testing.T) {
	s := newCheckzServer(sessionStore(), nil) // Repo unset
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithSession())

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful degradation)", rr.Code)
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rr.Body.String(), err)
	}
	if got["auth"] != "valid" {
		t.Errorf("auth = %v, want valid", got["auth"])
	}
	if _, ok := got["tenants"]; ok {
		t.Errorf("response unexpectedly contains tenants: %v", got)
	}
}

func TestCheckz_RepoError_BasicResponseNoTenants(t *testing.T) {
	repo := &fakeRepo{err: context.DeadlineExceeded}
	s := newCheckzServer(sessionStore(), repo)
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithSession())

	// A transient MT-DB error must NOT break the session check.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (lookup error degrades gracefully)", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rr.Body.String(), err)
	}
	if got["auth"] != "valid" {
		t.Errorf("auth = %v, want valid", got["auth"])
	}
	if _, ok := got["tenants"]; ok {
		t.Errorf("response unexpectedly contains tenants on lookup error: %v", got)
	}
}

// checkzServerWithOrigins builds a Server like newCheckzServer but with a
// configured CheckzAllowedOrigins allowlist for the Origin-reject tests.
func checkzServerWithOrigins(store login.SessionStore, repo tenantLookup, origins []string) *Server {
	s := newCheckzServer(store, repo)
	s.opts.CheckzAllowedOrigins = origins
	return s
}

// checkzReqWithOrigin is checkzReqWithSession plus an Origin header.
func checkzReqWithOrigin(origin string) *http.Request {
	req := checkzReqWithSession()
	req.Header.Set("Origin", origin)
	return req
}

// (a) No Origin header + configured allowlist -> normal 200 (same-origin /
// top-level navigation / non-browser callers omit Origin).
func TestCheckz_NoOrigin_Allowed(t *testing.T) {
	s := checkzServerWithOrigins(sessionStore(), &fakeRepo{}, []string{"https://app.example.net"})
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithSession()) // no Origin header

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no Origin header is allowed)", rr.Code)
	}
	if got := rr.Body.String(); !contains(got, `"auth":"valid"`) {
		t.Errorf("body %q missing \"auth\":\"valid\"", got)
	}
}

// (b) Origin == allowed app origin -> normal 200.
func TestCheckz_AllowedOrigin_Allowed(t *testing.T) {
	s := checkzServerWithOrigins(sessionStore(), &fakeRepo{}, []string{"https://app.example.net", "https://app2.example.net"})
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithOrigin("https://app2.example.net"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (allowed origin)", rr.Code)
	}
	if got := rr.Body.String(); !contains(got, `"auth":"valid"`) {
		t.Errorf("body %q missing \"auth\":\"valid\"", got)
	}
}

// (c) Origin == evil.com -> 403 before any session/PII processing.
func TestCheckz_DisallowedOrigin_403_NoPII(t *testing.T) {
	repo := &fakeRepo{user: &mtdb.User{
		ID: "u@example.com", Email: "u@example.com", Enabled: true,
		TenantSlugs: []string{"acme"},
	}}
	s := checkzServerWithOrigins(sessionStore(), repo, []string{"https://app.example.net"})
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithOrigin("https://evil.com"))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (disallowed origin)", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if body := rr.Body.String(); body != `{"error":"origin not allowed"}` {
		t.Errorf("body = %q, want origin-not-allowed error", body)
	}
	// No PII (email / user_id / tenant slug) may leak in the rejection body.
	for _, pii := range []string{"u@example.com", "acme", "auth", "valid"} {
		if contains(rr.Body.String(), pii) {
			t.Errorf("403 body %q leaked PII/session field %q", rr.Body.String(), pii)
		}
	}
}

// (e) Top-level browser navigation (Sec-Fetch-Mode: navigate) -> 403 before any
// session/PII processing, regardless of Origin/allowlist. Kills "browse to
// /checkz and read the session JSON".
func TestCheckz_Navigation_403_NoPII(t *testing.T) {
	repo := &fakeRepo{user: &mtdb.User{
		ID: "u@example.com", Email: "u@example.com", Enabled: true,
		TenantSlugs: []string{"acme"},
	}}
	s := checkzServerWithOrigins(sessionStore(), repo, []string{"https://app.example.net"})
	for _, hdr := range []struct{ k, v string }{
		{"Sec-Fetch-Mode", "navigate"},
		{"Sec-Fetch-Dest", "document"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/checkz", nil)
		req.Header.Set(hdr.k, hdr.v)
		rr := httptest.NewRecorder()
		s.handleCheckz(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s=%s: status = %d, want 403 (navigation block)", hdr.k, hdr.v, rr.Code)
		}
		for _, pii := range []string{"u@example.com", "acme", "valid"} {
			if contains(rr.Body.String(), pii) {
				t.Errorf("%s=%s: 403 body %q leaked PII/session field %q", hdr.k, hdr.v, rr.Body.String(), pii)
			}
		}
	}
}

// (f) The SPA's cross-origin fetch (Sec-Fetch-Mode: cors + allowed Origin) is NOT
// blocked by the navigation rule.
func TestCheckz_CorsFetch_NotBlockedByNavRule(t *testing.T) {
	s := checkzServerWithOrigins(sessionStore(), &fakeRepo{}, []string{"https://app2.example.net"})
	req := checkzReqWithOrigin("https://app2.example.net")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA cors fetch allowed)", rr.Code)
	}
}

// (d) Empty allowlist (unconfigured) + evil Origin -> allowed (fail-open on
// config absence; the edge CORS still applies).
func TestCheckz_EmptyAllowlist_EvilOrigin_Allowed(t *testing.T) {
	s := checkzServerWithOrigins(sessionStore(), &fakeRepo{}, nil) // empty allowlist
	rr := httptest.NewRecorder()
	s.handleCheckz(rr, checkzReqWithOrigin("https://evil.com"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty allowlist disables origin reject)", rr.Code)
	}
	if got := rr.Body.String(); !contains(got, `"auth":"valid"`) {
		t.Errorf("body %q missing \"auth\":\"valid\"", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
