// SPDX-License-Identifier: AGPL-3.0-only
package integration_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/scitrera-auth-go/internal/external"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb/msgpack"
	"github.com/scitrera/scitrera-auth-go/internal/resolver"
	"github.com/scitrera/scitrera-auth-go/internal/testdb"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestMachineReverseProxy(t *testing.T) {
	repo, dsn := testdb.New(t)
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("synthetic-machine-key-", 2)
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(token))
	tokenHash := hex.EncodeToString(mac.Sum(nil))
	if _, err := repo.DB().Exec(`INSERT INTO auth_proxy.api_tokens(token_hash,name,principal_type,created_by,workspace_patterns) VALUES($1,'synthetic','Service','synthetic-service',ARRAY['auth-app'])`, tokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO auth_proxy.acl_rules(principal_type,principal_id,resource_type,resource_id,access_level) VALUES('wildcard','_any_service','workspace','auth-app',20)`); err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(r.Header)
	}))
	defer backend.Close()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", "auth_proxy,public")
	u.RawQuery = q.Encode()
	addr := freeAddr(t)
	mtResolver, _ := resolver.New(resolver.Options{Repo: repo})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	cfg := &authproxy.Config{Mode: authproxy.ModeProxy, ListenAddr: addr, DBURL: u.String(), BackendURL: backend.URL, TenantID: "auth-app", TokenHMACKey: key, LogLevel: "error"}
	go func() { done <- authproxy.Run(ctx, cfg, authproxy.WithIdentityResolver(mtResolver)) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("proxy did not drain")
		}
	})
	ready(t, "http://"+addr)
	req, _ := http.NewRequest("GET", "http://"+addr+"/example?workspace_id=auth-app", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Scitrera-User", "spoof@example.com")
	req.Header.Set("X-Auth-User-ID", "spoof@example.com")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("machine proxy %s: %s", response.Status, body)
	}
	var headers http.Header
	if err = json.NewDecoder(response.Body).Decode(&headers); err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Auth-User-ID") != "synthetic-service" || headers.Get("X-Scitrera-User") == "spoof@example.com" {
		t.Fatalf("machine identity/spoof boundary: %v", headers)
	}
}
func ready(t *testing.T, address string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		resp, err := http.Get(address + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("listener not ready")
}
func TestSyntheticOIDCLoginTenantGateAndProxy(t *testing.T) {
	for _, autoAdd := range []bool{false, true} {
		name := "registered_member"
		if autoAdd {
			name = "automatic_enrollment"
		}
		t.Run(name, func(t *testing.T) { syntheticOIDC(t, autoAdd) })
	}
}
func syntheticOIDC(t *testing.T, autoAdd bool) {
	repo, dsn := testdb.New(t)
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec("INSERT INTO public.tenants(slug,name) VALUES('alpha','Alpha')"); err != nil {
		t.Fatal(err)
	}
	if autoAdd {
		if _, err := repo.DB().Exec("INSERT INTO public.tenant_domains(tenant_slug,domain) VALUES('alpha','example.com')"); err != nil {
			t.Fatal(err)
		}
		for key, value := range map[string]any{"auth:auto_add": true, "auth:checks:azure": map[string]any{"tid": "11111111-1111-4111-8111-111111111111"}} {
			raw, _ := msgpack.Encode(value)
			if _, err := repo.DB().Exec("INSERT INTO public.tenant_config(tenant_slug,key,value) VALUES('alpha',$1,$2)", key, raw); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		if _, err := repo.DB().Exec("INSERT INTO public.users(email,name) VALUES('person@example.com','Synthetic Person'); INSERT INTO public.user_tenants(user_id,tenant_id) SELECT u.id,t.id FROM public.users u CROSS JOIN public.tenants t"); err != nil {
			t.Fatal(err)
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(nil)
	defer issuer.Close()
	issuer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize", "token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/jwks", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "synthetic", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/authorize":
			target, _ := url.Parse(r.URL.Query().Get("redirect_uri"))
			q := target.Query()
			q.Set("state", r.URL.Query().Get("state"))
			q.Set("code", "synthetic-code")
			target.RawQuery = q.Encode()
			http.Redirect(w, r, target.String(), 302)
		case "/token":
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": issuer.URL, "aud": "synthetic-client", "sub": "synthetic-person", "email": "person@example.com", "email_verified": true, "name": "Synthetic Person", "tid": "11111111-1111-4111-8111-111111111111", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
			token.Header["kid"] = "synthetic"
			signed, _ := token.SignedString(key)
			json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access", "token_type": "Bearer", "expires_in": 3600, "id_token": signed})
		default:
			http.NotFound(w, r)
		}
	})
	externalAddr, internalAddr := freeAddr(t), freeAddr(t)
	for k, v := range map[string]string{"AUTH_PROXY_LOGIN_PROVIDERS": "azure", "AUTH_PROXY_LOGIN_AZURE_ISSUER": issuer.URL, "AUTH_PROXY_LOGIN_AZURE_CLIENT_ID": "synthetic-client", "AUTH_PROXY_LOGIN_AZURE_CLIENT_SECRET": "synthetic-secret", "AUTH_PROXY_LOGIN_AZURE_REDIRECT_URL": "http://" + externalAddr + "/auth/callback/azure", "AUTH_PROXY_SESSION_STORE": "jwt", "AUTH_PROXY_SESSION_JWT_SIGNING_KEY": strings.Repeat("synthetic-key-", 3), "AUTH_PROXY_SESSION_COOKIE_SECURE": "false", "AUTH_PROXY_SESSION_COOKIE_NAME": "auth_test_session", "AUTH_PROXY_SESSION_COOKIE_DOMAIN": ""} {
		t.Setenv(k, v)
	}
	mtResolver, _ := resolver.New(resolver.Options{Repo: repo})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", "auth_proxy,public")
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	cfg := &authproxy.Config{Mode: authproxy.ModeVerify, ListenAddr: internalAddr, DBURL: u.String(), TenantID: "auth-app", LogLevel: "error"}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("proxy failed to stop")
		}
	})
	front, err := external.New(external.Options{ListenAddr: externalAddr, TargetURL: "https://app.example.com", AllowedRedirectHosts: []string{"app.example.com"}, CheckzAllowedOrigins: []string{"https://app.example.com"}, ReturnCookieHMACKey: []byte(strings.Repeat("synthetic", 4)), Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- authproxy.Run(ctx, cfg, authproxy.WithIdentityResolver(mtResolver)) }()
	frontDone := make(chan error, 1)
	go func() { frontDone <- front.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		front.Stop(ctx)
		if err := <-frontDone; err != nil {
			t.Error(err)
		}
	})
	extURL, intURL := "http://"+externalAddr, "http://"+internalAddr
	ready(t, extURL)
	ready(t, intURL)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(address string) *http.Response {
		t.Helper()
		resp, err := client.Get(address)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	response := get(extURL + "/auth/login/azure?next=//attacker.example")
	if response.StatusCode != 302 {
		t.Fatal("login", response.Status)
	}
	authorize := response.Header.Get("Location")
	response.Body.Close()
	response = get(authorize)
	callback := response.Header.Get("Location")
	response.Body.Close()
	response = get(callback)
	if response.StatusCode != 302 || strings.Contains(response.Header.Get("Location"), "attacker") {
		t.Fatal("callback failed/open redirect", response.Status, response.Header)
	}
	response.Body.Close()
	response = get(intURL + "/auth/verify?workspace_id=auth-app&tenant_id=alpha")
	if response.StatusCode != 200 || response.Header.Get("X-Scitrera-Default-Tenant") != "alpha" {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("verify %s %s", response.Status, body)
	}
	response.Body.Close()
	if autoAdd {
		user, err := repo.GetUserWithTenants(context.Background(), "person@example.com")
		if err != nil || user == nil || len(user.TenantSlugs) != 1 || user.DefaultTenantSlug != "alpha" {
			t.Fatal("OIDC verification did not persist enrollment", user, err)
		}
	}
	response = get(intURL + "/auth/verify?workspace_id=auth-app&tenant_id=beta")
	if response.StatusCode != 403 {
		t.Fatal("cross-tenant admitted", response.Status)
	}
	response.Body.Close()
	// Public listener never exposes verification or operator endpoints.
	response = get(extURL + "/auth/verify")
	if response.StatusCode != 404 {
		t.Fatal("external verification exposed")
	}
	response.Body.Close()
	req, _ := http.NewRequest("GET", extURL+"/checkz", nil)
	req.Header.Set("Origin", "https://attacker.example")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 403 {
		t.Fatal("checkz origin accepted")
	}
	response.Body.Close()
	req, _ = http.NewRequest("GET", intURL+"/auth/verify-optional?workspace_id=auth-app", nil)
	req.Header.Set("X-Scitrera-User", "spoof@example.com")
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.Header.Get("X-Scitrera-User") != "" {
		t.Fatal("anonymous spoof survived", response.Status)
	}
	response.Body.Close()
	if _, err = repo.DB().Exec(`UPDATE public.users SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	response = get(intURL + "/auth/verify?workspace_id=auth-app&tenant_id=alpha")
	if response.StatusCode != 403 {
		t.Fatal("user disable not enforced", response.Status)
	}
	response.Body.Close()
	req, _ = http.NewRequest("POST", extURL+"/auth/logout", nil)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 302 && response.StatusCode != 303 {
		t.Fatal("logout failed", response.Status)
	}
	response.Body.Close()
	response = get(intURL + "/auth/verify?workspace_id=auth-app&tenant_id=alpha")
	if response.StatusCode != 401 {
		t.Fatal("logout cookie remained", response.Status)
	}
	response.Body.Close()
}
