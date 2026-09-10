// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"
)

// fakeStore is an in-memory login.SessionStore for tests.
type fakeStore struct {
	data map[string]*login.SessionData
}

func (f *fakeStore) Name() string { return "fake" }

func (f *fakeStore) New(_ context.Context, d *login.SessionData) (string, error) {
	id := "sid-" + d.UserID
	if f.data == nil {
		f.data = map[string]*login.SessionData{}
	}
	f.data[id] = d
	return id, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (*login.SessionData, error) {
	return f.data[id], nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	delete(f.data, id)
	return nil
}

// newTestServer builds a Server wired with a fake store and a couple of
// provider buttons, bypassing New() (which needs Redis + OIDC discovery).
func newTestServer(store login.SessionStore, opts Options) *Server {
	return &Server{
		store:     store,
		cookies:   login.CookieConfig{Name: "scitrera_session", Secure: false, SameSite: http.SameSiteLaxMode},
		providers: buildProviderButtons([]string{"azure", "google"}),
		opts:      opts,
	}
}

func TestLanding_NoSession_RendersProviders(t *testing.T) {
	s := newTestServer(&fakeStore{}, Options{TargetURL: "https://app.example.net"})

	rr := httptest.NewRecorder()
	s.handleLanding(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Sign in with Google", "Sign in with Microsoft", "/auth/login/google", "/auth/login/azure"} {
		if !strings.Contains(body, want) {
			t.Errorf("landing body missing %q", want)
		}
	}
}

func TestLanding_ValidSession_RedirectsToTarget(t *testing.T) {
	store := &fakeStore{data: map[string]*login.SessionData{
		"sid-1": {UserID: "1", Email: "a@b.com", ExpiresAt: time.Now().Add(time.Hour)},
	}}
	s := newTestServer(store, Options{TargetURL: "https://app.example.net"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
	rr := httptest.NewRecorder()
	s.handleLanding(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://app.example.net" {
		t.Errorf("Location = %q, want target", loc)
	}
}

func TestLanding_ValidSession_AllowlistedReturnCookieHonored(t *testing.T) {
	store := &fakeStore{data: map[string]*login.SessionData{
		"sid-1": {UserID: "1", ExpiresAt: time.Now().Add(time.Hour)},
	}}
	s := newTestServer(store, Options{
		TargetURL:            "https://app.example.net",
		AllowedRedirectHosts: []string{"docs.example.net"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
	req.AddCookie(&http.Cookie{Name: returnCookieName, Value: "https://docs.example.net/page"})
	rr := httptest.NewRecorder()
	s.handleLanding(rr, req)

	if got := rr.Header().Get("Location"); got != "https://docs.example.net/page" {
		t.Errorf("Location = %q, want allowlisted return url", got)
	}
}

func TestLanding_ValidSession_DisallowedReturnCookieFallsBackToTarget(t *testing.T) {
	store := &fakeStore{data: map[string]*login.SessionData{
		"sid-1": {UserID: "1", ExpiresAt: time.Now().Add(time.Hour)},
	}}
	s := newTestServer(store, Options{
		TargetURL:            "https://app.example.net",
		AllowedRedirectHosts: []string{"docs.example.net"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
	req.AddCookie(&http.Cookie{Name: returnCookieName, Value: "https://evil.example.com/x"})
	rr := httptest.NewRecorder()
	s.handleLanding(rr, req)

	if got := rr.Header().Get("Location"); got != "https://app.example.net" {
		t.Errorf("Location = %q, want fallback to target", got)
	}
}

func TestSanitizeRedirect(t *testing.T) {
	s := newTestServer(&fakeStore{}, Options{
		TargetURL:            "https://app.example.net",
		AllowedRedirectHosts: []string{"docs.example.net"},
	})
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", "", false},
		{"/app/page", "/app/page", true},
		{"//evil.com", "", false},
		{"/\\evil.com", "", false},
		{"https://docs.example.net/x", "https://docs.example.net/x", true},
		{"https://evil.example.com/x", "", false},
		{"ftp://docs.example.net", "", false},
		{"https://DOCS.example.net/x", "https://DOCS.example.net/x", true}, // case-insensitive host
	}
	for _, c := range cases {
		got, ok := s.sanitizeRedirect(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("sanitizeRedirect(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestRoutes_SurfaceShape(t *testing.T) {
	s := newTestServer(&fakeStore{}, Options{TargetURL: "https://app.example.net"})

	// Stub OSS login sub-mux: records the path it received.
	var gotPath string
	loginMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	mux := s.routes(loginMux)

	t.Run("auth/me is not exposed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/me", nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("/auth/me status = %d, want 404", rr.Code)
		}
	})

	t.Run("checkz is served by handleCheckz, not the login mux", func(t *testing.T) {
		gotPath = ""
		rr := httptest.NewRecorder()
		// No session cookie → handleCheckz returns 401 {"error":"no session"}
		// without ever touching the OSS login mux.
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/checkz", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("/checkz no-session status = %d, want 401", rr.Code)
		}
		if gotPath != "" {
			t.Errorf("/checkz forwarded to login mux (path %q), want handled locally", gotPath)
		}
		if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("/checkz Cache-Control = %q, want no-store", cc)
		}
	})

	t.Run("auth/login forwards to login mux", func(t *testing.T) {
		gotPath = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/login/azure", nil))
		if rr.Code != http.StatusOK || gotPath != "/auth/login/azure" {
			t.Errorf("/auth/login/azure -> (code %d, path %q), want (200, /auth/login/azure)", rr.Code, gotPath)
		}
	})

	t.Run("healthz ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rr.Code != http.StatusOK {
			t.Errorf("/healthz status = %d, want 200", rr.Code)
		}
	})

	t.Run("logout GET is rejected with 405", func(t *testing.T) {
		gotPath = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/logout", nil))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET /auth/logout status = %d, want 405", rr.Code)
		}
		if gotPath != "" {
			t.Errorf("GET /auth/logout forwarded to login mux (path %q), want not forwarded", gotPath)
		}
	})

	t.Run("logout POST forwards to login mux", func(t *testing.T) {
		gotPath = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
		if rr.Code == http.StatusMethodNotAllowed || gotPath != "/auth/logout" {
			t.Errorf("POST /auth/logout -> (code %d, path %q), want forwarded (not 405)", rr.Code, gotPath)
		}
	})
}

func TestSanitizeLoginNext_StripsProtocolRelative(t *testing.T) {
	s := newTestServer(&fakeStore{}, Options{TargetURL: "https://app.example.net"})

	var gotQuery string
	loginMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	})
	mux := s.routes(loginMux)

	t.Run("protocol-relative next is stripped", func(t *testing.T) {
		gotQuery = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/login/azure?next=//evil.com", nil))
		if strings.Contains(gotQuery, "next") {
			t.Errorf("forwarded query = %q, want next stripped", gotQuery)
		}
	})

	t.Run("backslash protocol-relative next is stripped", func(t *testing.T) {
		gotQuery = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/login/azure?next=/%5Cevil.com", nil))
		if strings.Contains(gotQuery, "next") {
			t.Errorf("forwarded query = %q, want next stripped", gotQuery)
		}
	})

	t.Run("valid relative next is preserved", func(t *testing.T) {
		gotQuery = ""
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/login/azure?next=/foo", nil))
		q, err := url.ParseQuery(gotQuery)
		if err != nil {
			t.Fatalf("parse forwarded query %q: %v", gotQuery, err)
		}
		if q.Get("next") != "/foo" {
			t.Errorf("forwarded next = %q, want /foo preserved", q.Get("next"))
		}
	})
}

func TestReturnCookie_HMAC(t *testing.T) {
	key := []byte("test-hmac-key-32-bytes-padding!!")
	store := &fakeStore{data: map[string]*login.SessionData{
		"sid-1": {UserID: "1", ExpiresAt: time.Now().Add(time.Hour)},
	}}
	s := newTestServer(store, Options{
		TargetURL:            "https://app.example.net",
		AllowedRedirectHosts: []string{"docs.example.net"},
		ReturnCookieHMACKey:  key,
	})

	t.Run("forged unsigned cookie is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
		req.AddCookie(&http.Cookie{Name: returnCookieName, Value: "https://docs.example.net/page"})
		rr := httptest.NewRecorder()
		s.handleLanding(rr, req)
		if got := rr.Header().Get("Location"); got != "https://app.example.net" {
			t.Errorf("Location = %q, want fallback to target (forged cookie rejected)", got)
		}
	})

	t.Run("properly signed cookie round-trips", func(t *testing.T) {
		signed := s.signReturnValue("https://docs.example.net/page")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "scitrera_session", Value: "sid-1"})
		req.AddCookie(&http.Cookie{Name: returnCookieName, Value: signed})
		rr := httptest.NewRecorder()
		s.handleLanding(rr, req)
		if got := rr.Header().Get("Location"); got != "https://docs.example.net/page" {
			t.Errorf("Location = %q, want signed return url honored", got)
		}
	})
}
