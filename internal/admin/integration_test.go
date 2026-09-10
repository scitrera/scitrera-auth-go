// SPDX-License-Identifier: AGPL-3.0-only
package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/admin"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
	"github.com/scitrera/scitrera-auth-go/internal/resolver"
	"github.com/scitrera/scitrera-auth-go/internal/testdb"
)

type client struct {
	t        *testing.T
	s        *admin.Server
	cookie   *http.Cookie
	csrf     string
	revision int64
}

func (c *client) call(method, path string, body any, status int) map[string]any {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "https://admin.example.test"+admin.Prefix+path, &buf)
	req.Header.Set("Origin", "https://admin.example.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", fmt.Sprintf(`"%d"`, c.revision))
	req.Header.Set("X-CSRF-Token", c.csrf)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, req)
	if w.Code != status {
		c.t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		c.t.Fatal(err)
	}
	if v, ok := out["revision"].(float64); ok {
		c.revision = int64(v)
	}
	if v, ok := out["csrf_token"].(string); ok {
		c.csrf = v
	}
	for _, cookie := range w.Result().Cookies() {
		c.cookie = cookie
	}
	return out
}
func setup(t *testing.T) (*client, *mtdb.Repo, string) {
	t.Helper()
	repo, _ := testdb.New(t)
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "operators.json")
	if err := admin.Bootstrap(path, "alice"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var creds admin.Credentials
	json.Unmarshal(raw, &creds)
	server, err := admin.New(admin.Options{Addr: "127.0.0.1:0", Origin: "https://admin.example.test", TokenFile: path, DB: repo.DB()})
	if err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, s: server}
	c.call("POST", "/session", map[string]string{"operator": "alice", "token": creds.Operators["alice"]}, 200)
	if !c.cookie.HttpOnly || !c.cookie.Secure || c.cookie.SameSite != http.SameSiteStrictMode || c.cookie.Domain != "" {
		t.Fatal("cookie boundary")
	}
	c.call("GET", "/status", nil, 200)
	return c, repo, path
}
func tenantBody(name string) map[string]any {
	return map[string]any{"slug": name, "name": strings.ToUpper(name), "enabled": true}
}
func userBody(enabled bool) map[string]any {
	return map[string]any{"email": "  Person@Example.COM  ", "name": "Person", "enabled": enabled}
}
func resolve(t *testing.T, r *resolver.Resolver, mail, provider, tenant string) *authproxy.ResolvedIdentity {
	t.Helper()
	in := authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: mail, Type: models.PrincipalUser}, Claims: map[string]any{"email": mail, "provider": provider, "tid": "expected"}, Request: httptest.NewRequest("GET", "/auth/verify?tenant_id="+tenant, nil)}
	got, err := r.Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestConfigurationLifecycleAndPropagation(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants", tenantBody("beta"), 200)
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": " Example.COM. "}, 200)
	c.call("POST", "/tenants/beta/domains", map[string]string{"domain": "example.com"}, 409)
	c.call("POST", "/users", userBody(true), 200)
	c.call("POST", "/users", userBody(true), 409)
	u := c.call("GET", "/users", nil, 200)["data"].([]any)[0].(map[string]any)
	id := u["id"].(string)
	if u["email"] != "person@example.com" {
		t.Fatal("email not normalized")
	}
	c.call("POST", "/users/"+id+"/memberships", map[string]string{"tenant_slug": "alpha"}, 200)
	c.call("POST", "/users/"+id+"/memberships", map[string]string{"tenant_slug": "beta"}, 200)
	body := userBody(true)
	body["default_tenant_slug"] = "alpha"
	c.call("PUT", "/users/"+id, body, 200)
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"providers": []string{"azure"}, "checks": map[string]any{"azure": map[string]any{"tid": []string{"expected"}}}}, 200)
	for i := 0; i < 2; i++ {
		r, err := resolver.New(resolver.Options{Repo: repo, CacheTTL: 5 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if got := resolve(t, r, "person@example.com", "azure", "alpha"); got.Reject != nil || got.DefaultTenantID != "alpha" {
			t.Fatal("enabled membership rejected", got.Reject)
		}
		if got := resolve(t, r, "unknown@example.com", "azure", "alpha"); got.Reject == nil {
			t.Fatal("domain association admitted a user while auto-add is off")
		}
		if got := resolve(t, r, "person@example.com", "google", "alpha"); got.Reject == nil {
			t.Fatal("wrong provider admitted")
		}
		// Warm both instances before disabling. See separate replay below.
		if i == 0 {
			t.Cleanup(func() {})
		}
	}
	r1, _ := resolver.New(resolver.Options{Repo: repo})
	r2, _ := resolver.New(resolver.Options{Repo: repo})
	for _, r := range []*resolver.Resolver{r1, r2} {
		if resolve(t, r, "person@example.com", "azure", "alpha").Reject != nil {
			t.Fatal("warm failed")
		}
	}
	body = userBody(false)
	c.call("PUT", "/users/"+id, body, 200)
	time.Sleep(1100 * time.Millisecond)
	for _, r := range []*resolver.Resolver{r1, r2} {
		if got := resolve(t, r, "person@example.com", "azure", "alpha"); got.Reject == nil || got.Reject.Code != "user_disabled" {
			t.Fatal("disable failed to propagate")
		}
	}
	body = userBody(true)
	c.call("PUT", "/users/"+id, body, 200)
	c.call("DELETE", "/users/"+id+"/memberships/alpha", nil, 200)
	u = c.call("GET", "/users/"+id, nil, 200)["data"].(map[string]any)
	if u["default_tenant_slug"] != "" || len(u["memberships"].([]any)) != 1 {
		t.Fatal("removal corrupted membership/default", u)
	}
	body["default_tenant_slug"] = "alpha"
	c.call("PUT", "/users/"+id, body, 400)
	time.Sleep(1100 * time.Millisecond)
	for _, r := range []*resolver.Resolver{r1, r2} {
		if resolve(t, r, "person@example.com", "azure", "alpha").Reject == nil {
			t.Fatal("removed membership admitted")
		}
		if resolve(t, r, "person@example.com", "azure", "beta").Reject != nil {
			t.Fatal("other membership removed")
		}
	}
	// Policy changes invalidate cached config values and cached absence.
	c.call("PUT", "/tenants/beta/auth", map[string]any{"checks": map[string]any{"azure": map[string]any{"tid": "different"}}}, 200)
	time.Sleep(1100 * time.Millisecond)
	for _, r := range []*resolver.Resolver{r1, r2} {
		if resolve(t, r, "person@example.com", "azure", "beta").Reject == nil {
			t.Fatal("changed claim restriction not applied")
		}
	}
	c.call("PUT", "/tenants/beta/auth", map[string]any{"providers": []string{}, "checks": map[string]any{"azure": nil}}, 200)
	p := c.call("GET", "/tenants/beta/auth", nil, 200)["data"].(map[string]any)
	if len(p["checks"].(map[string]any)) != 0 {
		t.Fatal("explicit clear failed")
	}
	// Stale editors conflict without partial writes.
	stale := c.revision
	c.call("POST", "/tenants", tenantBody("gamma"), 200)
	c.revision = stale
	c.call("PUT", "/tenants/beta/auth", map[string]any{"providers": []string{"google"}}, 409)
	c.call("GET", "/status", nil, 200)
	if _, err := repo.DB().Exec(`CREATE FUNCTION public.test_reject_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.key='auth:checks:broken' THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_policy_failure BEFORE INSERT ON public.tenant_config FOR EACH ROW EXECUTE FUNCTION public.test_reject_policy()`); err != nil {
		t.Fatal(err)
	}
	c.call("PUT", "/tenants/beta/auth", map[string]any{"providers": []string{"google"}, "checks": map[string]any{"broken": map[string]any{"tid": "expected"}}}, 500)
	p = c.call("GET", "/tenants/beta/auth", nil, 200)["data"].(map[string]any)
	if len(p["providers"].([]any)) != 0 {
		t.Fatal("failed policy write was not atomic")
	}
	var count int
	if err := repo.DB().QueryRow(`SELECT count(*) FROM public.auth_admin_audit WHERE operator='alice'`).Scan(&count); err != nil || count < 10 {
		t.Fatal("missing audit", err, count)
	}
	var audit string
	repo.DB().QueryRow(`SELECT string_agg(row_to_json(a)::text,'') FROM public.auth_admin_audit a`).Scan(&audit)
	if strings.Contains(audit, "expected") || strings.Contains(audit, "token") {
		t.Fatal("sensitive payload audited")
	}
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal("repeat migration", err)
	}
}
func TestOperatorBoundary(t *testing.T) {
	c, repo, path := setup(t)
	oldCookie, oldCSRF := c.cookie, c.csrf
	c.cookie = nil
	c.call("GET", "/tenants", nil, 401)
	c.cookie = &http.Cookie{Name: "aether_session", Value: "ordinary-user"}
	c.call("GET", "/users", nil, 401)
	c.cookie = oldCookie
	for _, route := range []string{"/tenants", "/users", "/status", "/tenants/alpha/auth", "/users/nope/memberships"} {
		cc := *c
		cc.cookie = nil
		cc.call("GET", route, nil, 401)
	}
	for _, origin := range []string{"", "https://attacker.example"} {
		req := httptest.NewRequest("POST", "https://admin.example.test"+admin.Prefix+"/tenants", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.AddCookie(oldCookie)
		w := httptest.NewRecorder()
		c.s.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("origin accepted", origin, w.Code)
		}
	}
	c.csrf = "wrong"
	c.call("POST", "/tenants", tenantBody("blocked"), 403)
	c.csrf = oldCSRF
	// Reload restores the same CSRF token and does not invalidate another tab.
	c.call("GET", "/session/me", nil, 200)
	if c.csrf != oldCSRF {
		t.Fatal("CSRF changed on reload")
	}
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("DELETE", "/session", nil, 200)
	c.cookie = oldCookie
	c.call("GET", "/status", nil, 401)
	for _, value := range []string{"", path + ".missing"} {
		if _, err := admin.New(admin.Options{Addr: "127.0.0.1:0", Origin: "https://admin.example.test", DB: repo.DB(), TokenFile: value}); err == nil {
			t.Fatal("missing credentials enabled administration")
		}
	}
	raw, _ := os.ReadFile(path)
	var creds admin.Credentials
	json.Unmarshal(raw, &creds)
	c.call("POST", "/session", map[string]string{"operator": "alice", "token": creds.Operators["alice"]}, 200)
	if _, err := repo.DB().Exec(`UPDATE public.auth_admin_sessions SET expires_at=CURRENT_TIMESTAMP - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/status", nil, 401)
	if err := admin.Bootstrap(path, "alice"); err == nil {
		t.Fatal("bootstrap overwrote file")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.New(admin.Options{Addr: "127.0.0.1:0", Origin: "https://admin.example.test", DB: repo.DB(), TokenFile: path}); err == nil {
		t.Fatal("public credential file accepted")
	}
}

func TestAuthScopedMetadataAndUnknownConfigSurvive(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	if _, err := repo.DB().Exec(`UPDATE public.tenants SET metadata='{"private_flag":"preserve","nullable":null,"logo":"https://example.com/logo.png"}'; INSERT INTO public.tenant_config(tenant_slug,key,value) VALUES('alpha','platform:untouched',decode('0102','hex'))`); err != nil {
		t.Fatal(err)
	}
	got := c.call("GET", "/tenants/alpha", nil, 200)["data"].(map[string]any)
	metadata := got["metadata"].(map[string]any)
	if _, ok := metadata["private_flag"]; ok {
		t.Fatal("unrelated metadata exposed")
	}
	c.call("PUT", "/tenants/alpha", map[string]any{"name": "Alpha updated", "enabled": true, "metadata": map[string]any{"logo": nil, "default_workspace": "workspace"}}, 200)
	var kept bool
	err := repo.DB().QueryRow(`SELECT metadata->>'private_flag'='preserve' AND metadata ? 'nullable' AND NOT metadata ? 'logo' FROM public.tenants WHERE slug='alpha'`).Scan(&kept)
	if err != nil || !kept {
		t.Fatal("unknown metadata/clear semantics", err)
	}
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": map[string]any{"azure": map[string]any{"custom": "preserve", "tid": "old"}, "other": map[string]any{"department": "research"}}}, 200)
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": map[string]any{"azure": map[string]any{"custom": "preserve"}}}, 200)
	policy := c.call("GET", "/tenants/alpha/auth", nil, 200)["data"].(map[string]any)
	if len(policy["checks"].(map[string]any)) != 2 {
		t.Fatal("omitted provider lost")
	}
	var raw []byte
	if err = repo.DB().QueryRow(`SELECT value FROM public.tenant_config WHERE key='platform:untouched'`).Scan(&raw); err != nil || string(raw) != string([]byte{1, 2}) {
		t.Fatal("unrelated config changed", err)
	}
}

func TestGoogleHostedDomainsAndRegisteredGuests(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
	c.call("POST", "/users", userBody(true), 200)
	id := c.call("GET", "/users", nil, 200)["data"].([]any)[0].(map[string]any)["id"].(string)
	c.call("POST", "/users/"+id+"/memberships", map[string]string{"tenant_slug": "alpha"}, 200)
	policy := map[string]any{"providers": []string{"google"}, "checks": map[string]any{"google": map[string]any{"hd": []string{" Example.COM. ", "second.example.org", ""}}}}
	c.call("PUT", "/tenants/alpha/auth", policy, 200)
	stored := c.call("GET", "/tenants/alpha/auth", nil, 200)["data"].(map[string]any)["checks"].(map[string]any)["google"].(map[string]any)["hd"].([]any)
	if len(stored) != 3 || stored[0] != "example.com" || stored[1] != "second.example.org" || stored[2] != "" {
		t.Fatal("hosted-domain options did not round trip", stored)
	}
	packed, err := repo.GetTenantConfigParam(context.Background(), "alpha", "auth:checks:google")
	if err != nil || packed.(map[string]any)["hd"].([]any)[2] != "" {
		t.Fatal("blank option lost in MessagePack", err)
	}
	r, _ := resolver.New(resolver.Options{Repo: repo})
	check := func(mail string, hd *string, allowed bool) {
		t.Helper()
		claims := map[string]any{"email": mail, "provider": "google"}
		if hd != nil {
			claims["hd"] = *hd
		}
		got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: mail, Type: models.PrincipalUser}, Claims: claims, Request: httptest.NewRequest("GET", "/auth/verify?tenant_id=alpha", nil)})
		if err != nil {
			t.Fatal(err)
		}
		if (got.Reject == nil) != allowed {
			t.Fatalf("mail=%s hd=%v allowed=%v want %v: %+v", mail, hd, got.Reject == nil, allowed, got.Reject)
		}
	}
	check("person@example.com", nil, true)
	// Even an associated email domain cannot use the blank option to admit an unknown account.
	check("unknown@example.com", nil, false)
	for _, hd := range []string{"example.com", "second.example.org"} {
		check("unknown@example.com", &hd, false)
	}
	wrong := "wrong.example.org"
	check("person@example.com", &wrong, false)
	var users, memberships int
	if err := repo.DB().QueryRow(`SELECT (SELECT count(*) FROM public.users), (SELECT count(*) FROM public.user_tenants)`).Scan(&users, &memberships); err != nil || users != 1 || memberships != 1 {
		t.Fatal("sign-in auto-added records", users, memberships, err)
	}
	// Reject malformed domain options, and retain nonempty rules for all other claims/providers.
	for _, checks := range []map[string]any{
		{"google": map[string]any{"hd": []any{"example.com", nil}}},
		{"google": map[string]any{"hd": []any{"example.com", true}}},
		{"google": map[string]any{"hd": "   "}},
		{"google": map[string]any{"hd": "*.example.com"}},
		{"google": map[string]any{"department": ""}},
		{"azure": map[string]any{"tid": []string{""}}},
		{"other": map[string]any{"hd": []string{""}}},
	} {
		c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": checks}, 400)
	}
	// Remove the guest option after warming caches; it must stop authorizing the member.
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": map[string]any{"google": map[string]any{"hd": []string{"example.com", "second.example.org"}}}}, 200)
	time.Sleep(1100 * time.Millisecond)
	check("person@example.com", nil, false)
	second := "second.example.org"
	check("person@example.com", &second, true)
}

func TestPublicDashboardNavigationKeepsAPIBoundary(t *testing.T) {
	c, repo, path := setup(t)
	server, err := admin.New(admin.Options{Addr: "127.0.0.1:0", Origin: "https://admin.example.test", TokenFile: path, DB: repo.DB(), UI: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("public sign-in shell")) })})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, origin, host string
		status                     int
	}{
		{"GET", "/", "", "admin.example.test", 303},
		{"GET", "/admin/", "", "admin.example.test", 200},
		{"HEAD", "/admin/assets/app.js", "", "admin.example.test", 200},
		{"GET", "/api/auth-admin/v1/status", "", "admin.example.test", 403},
		{"GET", "/api/auth-admin/v1/session/me", "", "admin.example.test", 403},
		{"POST", "/api/auth-admin/v1/session", "https://admin.example.test", "admin.example.test", 403},
		{"POST", "/admin/", "https://admin.example.test", "admin.example.test", 403},
		{"GET", "/admin/", "https://foreign.example.test", "admin.example.test", 403},
		{"GET", "/admin/", "", "foreign.example.test", 403},
	} {
		t.Run(tc.method+tc.path+tc.origin+tc.host, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(c.cookie)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestUserTenantAndSearchFilters(t *testing.T) {
	c, _, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants", tenantBody("beta"), 200)
	for _, u := range []map[string]any{
		{"email": "alice@example.com", "name": "Alice Research", "enabled": true},
		{"email": "bob@example.com", "name": "Bob Research", "enabled": false},
		{"email": "carol@example.com", "name": "Carol Operations", "enabled": true},
	} {
		c.call("POST", "/users", u, 200)
	}
	all := c.call("GET", "/users", nil, 200)["data"].([]any)
	alice, bob, carol := all[0].(map[string]any)["id"].(string), all[1].(map[string]any)["id"].(string), all[2].(map[string]any)["id"].(string)
	for _, member := range []struct{ id, tenant string }{{alice, "alpha"}, {alice, "beta"}, {bob, "alpha"}, {carol, "beta"}} {
		c.call("POST", "/users/"+member.id+"/memberships", map[string]string{"tenant_slug": member.tenant}, 200)
	}
	for _, tc := range []struct {
		query  string
		emails string
	}{
		{"tenant=alpha", "alice@example.com,bob@example.com"},
		{"tenant=beta", "alice@example.com,carol@example.com"},
		{"tenant=%20ALPHA%20&enabled=false", "bob@example.com"},
		{"tenant=alpha&q=RESEARCH&enabled=true", "alice@example.com"},
		{"tenant=alpha&limit=1&offset=1", "bob@example.com"},
		{"tenant=missing", ""},
		{"q=%25", ""},
		{"q=carol%40example.com", "carol@example.com"},
	} {
		got := c.call("GET", "/users?"+tc.query, nil, 200)["data"].([]any)
		var emails []string
		for _, row := range got {
			emails = append(emails, row.(map[string]any)["email"].(string))
		}
		if strings.Join(emails, ",") != tc.emails {
			t.Fatalf("filter %s got %v", tc.query, emails)
		}
	}
	c.call("GET", "/users?tenant=bad/slug", nil, 400)
	c.call("GET", "/users?enabled=maybe", nil, 400)
	tenant := c.call("GET", "/tenants?q=ALP&enabled=true", nil, 200)["data"].([]any)
	if len(tenant) != 1 || tenant[0].(map[string]any)["slug"] != "alpha" {
		t.Fatal("tenant filter", tenant)
	}
	// Filtering never edits membership and includes memberships in disabled tenants.
	c.call("PUT", "/tenants/alpha", map[string]any{"name": "Alpha", "enabled": false}, 200)
	if len(c.call("GET", "/users?tenant=alpha", nil, 200)["data"].([]any)) != 2 {
		t.Fatal("disabled tenant members hidden")
	}
	user := c.call("GET", "/users/"+alice, nil, 200)["data"].(map[string]any)
	if len(user["memberships"].([]any)) != 2 {
		t.Fatal("filter lost other memberships")
	}
}
