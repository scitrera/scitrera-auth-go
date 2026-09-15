// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
	"github.com/scitrera/scitrera-auth-go/internal/testdb"
)

type brandingRepo struct {
	fakeRepo
	lookup func(context.Context, string) (*mtdb.Tenant, error)
}

func (f *brandingRepo) GetTenantBySlug(ctx context.Context, slug string) (*mtdb.Tenant, error) {
	return f.lookup(ctx, slug)
}

func TestLoginTenantBranding(t *testing.T) {
	base := Branding{ProductName: "Global Portal", LogoURL: "https://global.example/logo.png", Tagline: "Welcome to the portal."}
	acme := &mtdb.Tenant{Slug: "acme", Name: "Acme & Partners", Logo: "https://acme.example/logo.png", Enabled: true}
	for _, tc := range []struct {
		name, query, defaultTenant, wantLookup, wantName, wantLogo string
		tenant                                                     *mtdb.Tenant
		err                                                        error
	}{
		{name: "no hint"},
		{name: "query", query: "?tenant=acme", wantLookup: "acme", tenant: acme, wantName: "Acme &amp; Partners", wantLogo: acme.Logo},
		{name: "normalized slug", query: "?tenant=%20Acme%20", wantLookup: "acme", tenant: acme, wantName: "Acme &amp; Partners", wantLogo: acme.Logo},
		{name: "default", defaultTenant: "acme", wantLookup: "acme", tenant: acme, wantName: "Acme &amp; Partners", wantLogo: acme.Logo},
		{name: "query overrides default", query: "?tenant=acme", defaultTenant: "other", wantLookup: "acme", tenant: acme, wantName: "Acme &amp; Partners", wantLogo: acme.Logo},
		{name: "empty bypasses default", query: "?tenant=", defaultTenant: "acme"},
		{name: "invalid bypasses default", query: "?tenant=../acme", defaultTenant: "acme"},
		{name: "malformed bypasses default", query: "?tenant=%ZZ", defaultTenant: "acme"},
		{name: "duplicate bypasses default", query: "?tenant=acme&tenant=other", defaultTenant: "acme"},
		{name: "long slug", query: "?tenant=" + strings.Repeat("a", 64)},
		{name: "unknown bypasses default", query: "?tenant=unknown", defaultTenant: "acme", wantLookup: "unknown"},
		{name: "disabled", query: "?tenant=acme", wantLookup: "acme", tenant: &mtdb.Tenant{Name: acme.Name, Logo: acme.Logo}},
		{name: "missing fields", query: "?tenant=acme", wantLookup: "acme", tenant: &mtdb.Tenant{Enabled: true}},
		{name: "name only", query: "?tenant=acme", wantLookup: "acme", tenant: &mtdb.Tenant{Name: acme.Name, Enabled: true}, wantName: "Acme &amp; Partners"},
		{name: "logo only", query: "?tenant=acme", wantLookup: "acme", tenant: &mtdb.Tenant{Logo: acme.Logo, Enabled: true}, wantLogo: acme.Logo},
		{name: "database error", query: "?tenant=acme", wantLookup: "acme", err: errors.New("database unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := newTestServer(&fakeStore{}, Options{Branding: base, DefaultTenant: tc.defaultTenant})
			s.repo = &brandingRepo{lookup: func(ctx context.Context, slug string) (*mtdb.Tenant, error) {
				calls++
				if slug != tc.wantLookup {
					t.Errorf("lookup = %q, want %q", slug, tc.wantLookup)
				}
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
					t.Error("lookup needs a bounded deadline")
				}
				return tc.tenant, tc.err
			}}
			for _, path := range []string{"/", "/login"} {
				rr := httptest.NewRecorder()
				s.handleLanding(rr, httptest.NewRequest(http.MethodGet, path+tc.query, nil))
				if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("response = %d, headers = %v", rr.Code, rr.Header())
				}
				name, logo := tc.wantName, tc.wantLogo
				if name == "" {
					name = base.ProductName
				}
				if logo == "" {
					logo = base.LogoURL
				}
				for _, want := range []string{"<h1>" + name + "</h1>", `src="` + logo + `"`, base.Tagline, `href="/auth/login/azure"`, `href="/auth/login/google"`, `Powered by <a href="https://scitrera.ai">scitrera.ai</a>`} {
					if !strings.Contains(rr.Body.String(), want) {
						t.Errorf("landing missing %q", want)
					}
				}
				if len(rr.Result().Cookies()) != 0 {
					t.Error("branding hint must not set cookies")
				}
			}
			wantCalls := 0
			if tc.wantLookup != "" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Errorf("lookup calls = %d, want %d", calls, wantCalls)
			}
			if s.opts.Branding != base {
				t.Error("request mutated global branding")
			}
		})
	}
}

func TestLoginBrandingRejectsUnsafeTenantLogos(t *testing.T) {
	for _, logo := range []string{"javascript:alert(1)", "data:image/svg+xml,test", "http://example.com/logo.png", "//example.com/logo.png", "https://user:password@example.com/logo.png", "https://", "https://example.com/\nlogo.png"} {
		t.Run(logo, func(t *testing.T) {
			s := newTestServer(&fakeStore{}, Options{})
			s.repo = &brandingRepo{lookup: func(context.Context, string) (*mtdb.Tenant, error) {
				return &mtdb.Tenant{Enabled: true, Name: `<script>alert(1)</script>`, Logo: logo}, nil
			}}
			rr := httptest.NewRecorder()
			s.handleLanding(rr, httptest.NewRequest(http.MethodGet, "/?tenant=acme", nil))
			body := rr.Body.String()
			if !strings.Contains(body, `src="https://scitrera.com/logo2.png"`) || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
				t.Fatal("tenant branding was not safely rendered")
			}
		})
	}
}

func TestLoginBrandingDefaultsAndCancellation(t *testing.T) {
	for _, key := range []string{"SCITRERA_AUTH_BRAND_NAME", "SCITRERA_AUTH_BRAND_LOGO_URL", "SCITRERA_AUTH_BRAND_TAGLINE"} {
		t.Setenv(key, "")
	}
	s := newTestServer(&fakeStore{}, Options{DefaultTenant: "acme"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	base := s.brandingForRequest(r)
	if base != BrandingFromEnv() || base.ProductName != "scitrera.ai" || base.LogoURL != "https://scitrera.com/logo2.png" {
		t.Fatalf("unexpected defaults without repository: %+v", base)
	}
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	s.repo = &brandingRepo{lookup: func(ctx context.Context, _ string) (*mtdb.Tenant, error) { return nil, ctx.Err() }}
	if got := s.brandingForRequest(r.WithContext(ctx)); got != base {
		t.Fatalf("cancellation did not fall back: %+v", got)
	}
	t.Setenv("SCITRERA_AUTH_BRAND_NAME", "Custom Portal")
	t.Setenv("SCITRERA_AUTH_BRAND_LOGO_URL", "https://custom.example/logo.png")
	t.Setenv("SCITRERA_AUTH_BRAND_TAGLINE", "Custom welcome")
	s.opts.Branding = BrandingFromEnv()
	if got := s.brandingForRequest(r.WithContext(ctx)); got != (Branding{ProductName: "Custom Portal", LogoURL: "https://custom.example/logo.png", Tagline: "Custom welcome"}) {
		t.Fatalf("global branding overrides lost: %+v", got)
	}
}

func TestTenantHintPreservesReturnDestination(t *testing.T) {
	store := &fakeStore{}
	s := newTestServer(store, Options{TargetURL: "https://app.example.com", AllowedRedirectHosts: []string{"app.example.com"}, ReturnCookieHMACKey: []byte("synthetic-return-key")})
	s.repo = &brandingRepo{lookup: func(context.Context, string) (*mtdb.Tenant, error) {
		return &mtdb.Tenant{Name: "Acme", Enabled: true}, nil
	}}
	rr := httptest.NewRecorder()
	s.handleLanding(rr, httptest.NewRequest(http.MethodGet, "/login?tenant=acme&rd=https%3A%2F%2Fapp.example.com%2Fworkspace", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code)
	}
	returnCookies := rr.Result().Cookies()
	if len(returnCookies) != 1 || returnCookies[0].Name != returnCookieName {
		t.Fatal("expected only the return cookie")
	}
	// After OAuth, the tenant hint must neither trigger a branding lookup nor
	// alter the previously captured destination or the session's identity.
	s.repo = &brandingRepo{lookup: func(context.Context, string) (*mtdb.Tenant, error) {
		t.Fatal("authenticated landing must not look up branding")
		return nil, nil
	}}
	id, _ := store.New(context.Background(), &login.SessionData{UserID: "user-1"})
	r := httptest.NewRequest(http.MethodGet, "/?tenant=other", nil)
	r.AddCookie(returnCookies[0])
	r.AddCookie(&http.Cookie{Name: s.cookies.Name, Value: id})
	rr = httptest.NewRecorder()
	s.handleLanding(rr, r)
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "https://app.example.com/workspace" {
		t.Fatalf("unexpected redirect: %d %v", rr.Code, rr.Header())
	}
}

func TestLoginBrandingUsesStoredTenantProfile(t *testing.T) {
	repo, _ := testdb.New(t)
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO public.tenants(slug,name,enabled,metadata) VALUES('acme','Acme',true,'{"logo":"https://acme.example/logo.png","private_field":"must-not-appear"}')`); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(&fakeStore{}, Options{DefaultTenant: "acme"})
	s.repo = repo
	check := func(path, name, logo string) {
		t.Helper()
		rr := httptest.NewRecorder()
		s.handleLanding(rr, httptest.NewRequest(http.MethodGet, path, nil))
		body := rr.Body.String()
		if !strings.Contains(body, "<h1>"+name+"</h1>") || !strings.Contains(body, `src="`+logo+`"`) || strings.Contains(body, "must-not-appear") {
			t.Fatal("stored tenant profile did not render as expected")
		}
	}
	check("/login", "Acme", "https://acme.example/logo.png")
	check("/login?tenant=acme", "Acme", "https://acme.example/logo.png")
	check("/login?tenant=unknown", "scitrera.ai", "https://scitrera.com/logo2.png")
	if _, err := repo.DB().Exec(`UPDATE public.tenants SET name='Acme Updated',metadata='{}' WHERE slug='acme'`); err != nil {
		t.Fatal(err)
	}
	check("/login", "Acme Updated", "https://scitrera.com/logo2.png")
	if _, err := repo.DB().Exec(`UPDATE public.tenants SET enabled=false WHERE slug='acme'`); err != nil {
		t.Fatal(err)
	}
	check("/login", "scitrera.ai", "https://scitrera.com/logo2.png")
}
