// SPDX-License-Identifier: AGPL-3.0-only
package loginproviders

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"golang.org/x/oauth2"
)

const tenantA = "11111111-1111-4111-8111-111111111111"
const tenantB = "22222222-2222-4222-8222-222222222222"

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "login.microsoftonline.com" {
		return nil, fmt.Errorf("unexpected host %s", req.URL.Host)
	}
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	copy.URL = &u
	copy.Host = u.Host
	return r.base.RoundTrip(copy)
}

type microsoftFixture struct {
	key                     *rsa.PrivateKey
	server                  *httptest.Server
	ctx                     context.Context
	client                  *http.Client
	mu                      sync.Mutex
	keyID, keyIssuer, token string
	metadataEdit            func(map[string]any)
	keysRequests            atomic.Int32
}

func microsoftTestServer(t *testing.T) *microsoftFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &microsoftFixture{key: key, keyID: "key-one", keyIssuer: microsoftIssuerTemplate}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			authority := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
			m := map[string]any{"issuer": microsoftIssuerTemplate, "authorization_endpoint": microsoftOrigin + "/" + authority + "/oauth2/v2.0/authorize", "token_endpoint": microsoftOrigin + "/" + authority + "/oauth2/v2.0/token", "jwks_uri": microsoftOrigin + "/" + authority + "/discovery/v2.0/keys"}
			if f.metadataEdit != nil {
				f.metadataEdit(m)
			}
			json.NewEncoder(w).Encode(m)
		case strings.HasSuffix(r.URL.Path, "/discovery/v2.0/keys"):
			f.keysRequests.Add(1)
			raw, _ := json.Marshal(jose.JSONWebKey{Key: &f.key.PublicKey, KeyID: f.keyID, Algorithm: "RS256", Use: "sig"})
			var k map[string]any
			json.Unmarshal(raw, &k)
			k["issuer"] = f.keyIssuer
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{k}})
		case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token"):
			json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access", "token_type": "Bearer", "id_token": f.token})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	target, _ := url.Parse(f.server.URL)
	f.client = &http.Client{Transport: rewriteTransport{target: target, base: http.DefaultTransport}}
	f.ctx = oidc.ClientContext(context.Background(), f.client)
	return f
}
func (f *microsoftFixture) provider(t *testing.T, authority string) *login.Provider {
	t.Helper()
	p, err := New(f.ctx, login.ProviderConfig{Name: "azure", IssuerURL: microsoftOrigin + "/" + authority + "/v2.0", ClientID: "synthetic-client", ClientSecret: "synthetic-secret", RedirectURL: "https://example.invalid/callback"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *microsoftFixture) sign(t *testing.T, tid string, edit func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{"iss": microsoftOrigin + "/" + tid + "/v2.0", "tid": tid, "aud": "synthetic-client", "sub": "synthetic-subject", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": "expected-nonce"}
	if edit != nil {
		edit(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = f.keyID
	raw, err := token.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMicrosoftTenantIssuerAndTokenChecks(t *testing.T) {
	f := microsoftTestServer(t)
	p := f.provider(t, "organizations")
	cases := []struct {
		name, tid string
		edit      func(jwt.MapClaims)
		ok        bool
	}{
		{name: "first organization", tid: tenantA, ok: true},
		{name: "second organization", tid: tenantB, ok: true},
		{name: "different issuer tenant", tid: tenantA, edit: func(c jwt.MapClaims) { c["iss"] = microsoftOrigin + "/" + tenantB + "/v2.0" }},
		{name: "wrong host", tid: tenantA, edit: func(c jwt.MapClaims) { c["iss"] = "https://example.invalid/" + tenantA + "/v2.0" }},
		{name: "issuer suffix attack", tid: tenantA, edit: func(c jwt.MapClaims) { c["iss"] = microsoftOrigin + "/" + tenantA + "/v2.0/extra" }},
		{name: "missing tid", tid: tenantA, edit: func(c jwt.MapClaims) { delete(c, "tid") }},
		{name: "non-string tid", tid: tenantA, edit: func(c jwt.MapClaims) { c["tid"] = 123 }},
		{name: "malformed tid", tid: "../../example.invalid"},
		{name: "nil GUID", tid: "00000000-0000-0000-0000-000000000000"},
		{name: "urn GUID", tid: "urn:uuid:" + tenantA},
		{name: "wrong audience", tid: tenantA, edit: func(c jwt.MapClaims) { c["aud"] = "other-client" }},
		{name: "missing subject", tid: tenantA, edit: func(c jwt.MapClaims) { delete(c, "sub") }},
		{name: "missing expiration", tid: tenantA, edit: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "expired", tid: tenantA, edit: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{name: "not yet valid", tid: tenantA, edit: func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }},
		{name: "personal account", tid: personalTenant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.Verifier.Verify(f.ctx, f.sign(t, tc.tid, tc.edit))
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%v, want %v: %v", err == nil, tc.ok, err)
			}
		})
	}
	if got := f.keysRequests.Load(); got != 1 {
		t.Fatalf("keys requested %d times; valid tenants must share cache", got)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	raw := f.sign(t, tenantA, nil)
	token, _, _ := new(jwt.Parser).ParseUnverified(raw, jwt.MapClaims{})
	bad, _ := token.SignedString(other)
	if _, err := p.Verifier.Verify(f.ctx, bad); err == nil {
		t.Fatal("wrong signing key accepted")
	}
	if _, err := p.Verifier.Verify(f.ctx, strings.Repeat("x", 65537)); err == nil {
		t.Fatal("oversized token accepted")
	}
	if got := f.keysRequests.Load(); got != 1 {
		t.Fatalf("bad signature forced key refresh: %d", got)
	}
}

func TestMicrosoftSigningKeyIssuerScope(t *testing.T) {
	for _, tc := range []struct {
		scope, authority, tid string
		ok                    bool
	}{
		{microsoftIssuerTemplate, "organizations", tenantA, true},
		{microsoftOrigin + "/" + tenantA + "/v2.0", "organizations", tenantA, true},
		{microsoftOrigin + "/" + tenantB + "/v2.0", "organizations", tenantA, false},
		{microsoftOrigin + "/" + personalTenant + "/v2.0", "organizations", tenantA, false},
		{"https://example.invalid/{tenantid}/v2.0", "organizations", tenantA, false},
		{"", "organizations", tenantA, false},
		{microsoftOrigin + "/" + personalTenant + "/v2.0", "common", personalTenant, true},
	} {
		t.Run(tc.scope+tc.authority, func(t *testing.T) {
			f := microsoftTestServer(t)
			f.keyIssuer = tc.scope
			p := f.provider(t, tc.authority)
			_, err := p.Verifier.Verify(f.ctx, f.sign(t, tc.tid, nil))
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%v want=%v: %v", err == nil, tc.ok, err)
			}
		})
	}
}

func TestMicrosoftDiscoveryFailsClosed(t *testing.T) {
	for _, field := range []string{"issuer", "authorization_endpoint", "token_endpoint", "jwks_uri"} {
		t.Run(field, func(t *testing.T) {
			f := microsoftTestServer(t)
			f.metadataEdit = func(m map[string]any) { m[field] = "https://example.invalid/foreign" }
			_, err := New(f.ctx, login.ProviderConfig{Name: "azure", IssuerURL: microsoftOrigin + "/organizations/v2.0", ClientID: "synthetic-client", RedirectURL: "https://example.invalid/callback"})
			if err == nil {
				t.Fatal("unexpected metadata accepted")
			}
		})
	}
}

func TestMicrosoftRotationConcurrentCacheAndRefreshLimit(t *testing.T) {
	f := microsoftTestServer(t)
	p := f.provider(t, "organizations")
	v := p.Verifier.(*microsoftVerifier)
	raw := f.sign(t, tenantA, nil)
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := v.Verify(f.ctx, raw); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := f.keysRequests.Load(); got != 1 {
		t.Fatalf("concurrent discovery fetched keys %d times", got)
	}
	f.mu.Lock()
	f.keyID = "rotated-key"
	f.mu.Unlock()
	rotated := f.sign(t, tenantA, nil)
	if _, err := v.Verify(f.ctx, rotated); err == nil {
		t.Fatal("unknown key accepted during refresh cooldown")
	}
	v.keys.mu.Lock()
	v.keys.lastAttempt = time.Now().Add(-2 * time.Minute)
	v.keys.mu.Unlock()
	if _, err := v.Verify(f.ctx, rotated); err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if got := f.keysRequests.Load(); got != 2 {
		t.Fatalf("rotation fetched keys %d times", got)
	}
	v.keys.mu.Lock()
	v.keys.lastSuccess = time.Now().Add(-25 * time.Hour)
	v.keys.lastAttempt = time.Now().Add(-2 * time.Minute)
	v.keys.mu.Unlock()
	if _, err := v.Verify(f.ctx, rotated); err != nil {
		t.Fatalf("periodic refresh failed: %v", err)
	}
	if got := f.keysRequests.Load(); got != 3 {
		t.Fatalf("stale key cache not refreshed: %d", got)
	}
}

func TestMicrosoftCallbackNonceAndTenantAllowlist(t *testing.T) {
	f := microsoftTestServer(t)
	p := f.provider(t, "organizations")
	p.Config.AllowedTenantIDs = []string{tenantA}
	for _, tc := range []struct {
		name, tid, nonce string
		ok               bool
	}{{"valid", tenantA, "expected-nonce", true}, {"nonce mismatch", tenantA, "wrong", false}, {"missing expected nonce", tenantA, "", false}, {"disallowed tenant", tenantB, "expected-nonce", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f.mu.Lock()
			f.token = f.sign(t, tc.tid, nil)
			f.mu.Unlock()
			_, _, err := p.VerifyCallbackWithNonce(context.WithValue(f.ctx, oauth2.HTTPClient, f.client), "synthetic-code", tc.nonce)
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%v want=%v: %v", err == nil, tc.ok, err)
			}
		})
	}
}
