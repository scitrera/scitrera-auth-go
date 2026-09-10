// SPDX-License-Identifier: AGPL-3.0-only
package admin_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb/msgpack"
	"github.com/scitrera/scitrera-auth-go/internal/resolver"
)

const organization = "11111111-1111-4111-8111-111111111111"

func autoPolicy(enabled bool) map[string]any {
	return map[string]any{"auto_add": enabled, "providers": []string{"google", "azure"}, "checks": map[string]any{
		"google": map[string]any{"hd": []string{"example.com", "second.example.org", ""}},
		"azure":  map[string]any{"tid": organization},
	}}
}
func autoInput(email, provider, tenant string) authproxy.ResolverInput {
	return authproxy.ResolverInput{
		Method: "session", Identity: models.Identity{ID: email, Type: models.PrincipalUser},
		Claims:  map[string]any{"email": email, "provider": provider, "hd": "example.com", "email_verified": true, "tid": organization, "name": "Automatic Person"},
		Request: httptest.NewRequest("GET", "/auth/verify?tenant_id="+tenant, nil),
	}
}
func checkAuto(t *testing.T, r *resolver.Resolver, input authproxy.ResolverInput, allowed bool) *authproxy.ResolvedIdentity {
	t.Helper()
	got, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if (got.Reject == nil) != allowed {
		t.Fatalf("allowed=%v want %v: %+v", got.Reject == nil, allowed, got.Reject)
	}
	return got
}
func userCount(t *testing.T, repo *mtdb.Repo, email string) int {
	t.Helper()
	var n int
	if err := repo.DB().QueryRow("SELECT count(*) FROM public.users WHERE email=$1", email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAutoAddConfigurationEnrollmentAndRevocation(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
	if c.call("GET", "/tenants/alpha/auth", nil, 200)["data"].(map[string]any)["auto_add"] != false {
		t.Fatal("auto-add default is not off")
	}
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": "true"}, 400)
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": 1}, 400)
	c.call("PUT", "/tenants/alpha/auth", autoPolicy(false), 200)
	r1, _ := resolver.New(resolver.Options{Repo: repo})
	r2, _ := resolver.New(resolver.Options{Repo: repo})
	input := autoInput("new@example.com", "google", "alpha")
	checkAuto(t, r1, input, false)
	checkAuto(t, r2, input, false)
	if userCount(t, repo, "new@example.com") != 0 {
		t.Fatal("off created user")
	}
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": true}, 200)
	if c.call("GET", "/tenants/alpha/auth", nil, 200)["data"].(map[string]any)["auto_add"] != true {
		t.Fatal("flag did not round trip")
	}
	packed, err := repo.GetTenantConfigParam(context.Background(), "alpha", "auth:auto_add")
	if err != nil || packed != true {
		t.Fatal("MessagePack boolean missing", err)
	}
	// Omitted/null fields preserve the explicit flag and unrelated provider rules.
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": nil}, 200)
	if c.call("GET", "/tenants/alpha/auth", nil, 200)["data"].(map[string]any)["auto_add"] != true {
		t.Fatal("null changed flag")
	}
	time.Sleep(1100 * time.Millisecond)
	checkAuto(t, r1, input, true)
	checkAuto(t, r2, input, true)
	user, err := repo.GetUserWithTenants(context.Background(), "new@example.com")
	if err != nil || user == nil || !user.Enabled || user.DefaultTenantSlug != "alpha" || len(user.TenantSlugs) != 1 {
		t.Fatal("enrollment incomplete", user, err)
	}
	var audits int
	if err := repo.DB().QueryRow("SELECT count(*) FROM public.auth_admin_audit WHERE action='auto_add'").Scan(&audits); err != nil || audits != 1 {
		t.Fatal("duplicate/missing enrollment audit", audits, err)
	}
	c.call("GET", "/status", nil, 200)
	// Real persisted enrollment is immediately visible through the filtered admin API.
	users := c.call("GET", "/users?tenant=alpha&q=new@example.com", nil, 200)["data"].([]any)
	if len(users) != 1 {
		t.Fatal("auto-added user absent from tenant user list")
	}
	// Warm true policy, then disable. Even inside the cache propagation interval,
	// the transaction must observe the committed false and insert nothing.
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": false}, 200)
	checkAuto(t, r1, autoInput("later@example.com", "google", "alpha"), false)
	if userCount(t, repo, "later@example.com") != 0 {
		t.Fatal("stale true flag enrolled user")
	}
	checkAuto(t, r1, input, true) // Turning off enrollment does not remove members.
}

func TestAutoAddConcurrentAndExistingUsers(t *testing.T) {
	c, repo, _ := setup(t)
	for _, slug := range []string{"alpha", "beta"} {
		c.call("POST", "/tenants", tenantBody(slug), 200)
	}
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
	c.call("PUT", "/tenants/alpha/auth", autoPolicy(true), 200)
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := resolver.New(resolver.Options{Repo: repo})
			if err == nil {
				var got *authproxy.ResolvedIdentity
				got, err = r.Resolve(context.Background(), autoInput("concurrent@example.com", "google", "alpha"))
				if err == nil && got.Reject != nil {
					err = fmt.Errorf("rejected: %+v", got.Reject)
				}
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var users, members, audits int
	if err := repo.DB().QueryRow("SELECT (SELECT count(*) FROM public.users), (SELECT count(*) FROM public.user_tenants), (SELECT count(*) FROM public.auth_admin_audit WHERE action='auto_add')").Scan(&users, &members, &audits); err != nil || users != 1 || members != 1 || audits != 1 {
		t.Fatal("concurrent enrollment was not idempotent", users, members, audits, err)
	}
	c.call("GET", "/status", nil, 200)
	c.call("POST", "/users", map[string]any{"email": "existing@example.com", "name": "Existing Name", "enabled": true}, 200)
	id := c.call("GET", "/users?q=existing@example.com", nil, 200)["data"].([]any)[0].(map[string]any)["id"].(string)
	c.call("POST", "/users/"+id+"/memberships", map[string]string{"tenant_slug": "beta"}, 200)
	c.call("PUT", "/users/"+id, map[string]any{"email": "existing@example.com", "name": "Existing Name", "enabled": true, "default_tenant_slug": "beta"}, 200)
	r, _ := resolver.New(resolver.Options{Repo: repo})
	checkAuto(t, r, autoInput("existing@example.com", "azure", "alpha"), true)
	user, _ := repo.GetUserWithTenants(context.Background(), "existing@example.com")
	if user.Name != "Existing Name" || user.DefaultTenantSlug != "beta" || len(user.TenantSlugs) != 2 {
		t.Fatal("existing user overwritten", user)
	}
	// Repeated verification after enrollment does not write/bump the revision.
	before, _ := repo.Revision(context.Background())
	checkAuto(t, r, autoInput("existing@example.com", "azure", "alpha"), true)
	after, _ := repo.Revision(context.Background())
	if before != after {
		t.Fatal("repeat enrollment wrote again")
	}
}

func TestAutoAddRejectsIneligibleAccountsWithoutWrites(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	for _, domain := range []string{"example.com", "gmail.com"} {
		c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": domain}, 200)
	}
	c.call("PUT", "/tenants/alpha/auth", autoPolicy(true), 200)
	cases := []struct {
		name   string
		change func(*authproxy.ResolverInput)
	}{
		{"missing hosted domain", func(i *authproxy.ResolverInput) { delete(i.Claims, "hd") }},
		{"blank hosted domain", func(i *authproxy.ResolverInput) { i.Claims["hd"] = "" }},
		{"null hosted domain", func(i *authproxy.ResolverInput) { i.Claims["hd"] = nil }},
		{"wrong hosted domain", func(i *authproxy.ResolverInput) { i.Claims["hd"] = "wrong.example.org" }},
		{"unverified email", func(i *authproxy.ResolverInput) { i.Claims["email_verified"] = false }},
		{"string verified email", func(i *authproxy.ResolverInput) { i.Claims["email_verified"] = "true" }},
		{"missing provider", func(i *authproxy.ResolverInput) { delete(i.Claims, "provider") }},
		{"unconfigured provider", func(i *authproxy.ResolverInput) { i.Claims["provider"] = "other" }},
		{"wrong tenant", func(i *authproxy.ResolverInput) {
			i.Request = httptest.NewRequest("GET", "/auth/verify?tenant_id=other", nil)
		}},
		{"wrong Entra organization", func(i *authproxy.ResolverInput) {
			i.Claims["provider"] = "azure"
			i.Claims["tid"] = "22222222-2222-4222-8222-222222222222"
		}},
	}
	for n, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := autoInput(fmt.Sprintf("denied%d@example.com", n), "google", "alpha")
			tc.change(&input)
			r, _ := resolver.New(resolver.Options{Repo: repo})
			checkAuto(t, r, input, false)
			if userCount(t, repo, input.Identity.ID) != 0 {
				t.Fatal("denied attempt wrote a user")
			}
		})
	}
	c.call("POST", "/users", map[string]any{"email": "disabled@example.com", "name": "Disabled", "enabled": false}, 200)
	r, _ := resolver.New(resolver.Options{Repo: repo})
	checkAuto(t, r, autoInput("disabled@example.com", "google", "alpha"), false)
	disabled, _ := repo.GetUserWithTenants(context.Background(), "disabled@example.com")
	if disabled.Enabled || len(disabled.TenantSlugs) != 0 {
		t.Fatal("disabled user changed")
	}
	// Explicitly selected Gmail users still work; unknown Gmail accounts never enroll.
	c.call("POST", "/users", map[string]any{"email": "selected@gmail.com", "name": "Selected", "enabled": true}, 200)
	id := c.call("GET", "/users?q=selected@gmail.com", nil, 200)["data"].([]any)[0].(map[string]any)["id"].(string)
	c.call("POST", "/users/"+id+"/memberships", map[string]string{"tenant_slug": "alpha"}, 200)
	r, _ = resolver.New(resolver.Options{Repo: repo})
	for _, email := range []string{"selected@gmail.com", "unknown@gmail.com"} {
		input := autoInput(email, "google", "alpha")
		delete(input.Claims, "hd")
		checkAuto(t, r, input, email == "selected@gmail.com")
	}
	// Removing the hd rule does not make unhosted accounts eligible.
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": map[string]any{"google": nil}}, 200)
	r, _ = resolver.New(resolver.Options{Repo: repo})
	input := autoInput("unknown@gmail.com", "google", "alpha")
	delete(input.Claims, "hd")
	checkAuto(t, r, input, false)
	checkAuto(t, r, autoInput("norule@example.com", "google", "alpha"), false)
}

func TestAutoAddRollbackAndPolicyChangeWhileWaiting(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
	c.call("PUT", "/tenants/alpha/auth", autoPolicy(true), 200)
	if _, err := repo.DB().Exec("CREATE FUNCTION public.fail_membership() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic membership failure'; END $$; CREATE TRIGGER fail_membership BEFORE INSERT ON public.user_tenants FOR EACH ROW EXECUTE FUNCTION public.fail_membership()"); err != nil {
		t.Fatal(err)
	}
	r, _ := resolver.New(resolver.Options{Repo: repo})
	input := autoInput("rollback@example.com", "google", "alpha")
	before, _ := repo.Revision(context.Background())
	if _, err := r.Resolve(context.Background(), input); err == nil {
		t.Fatal("injected failure was hidden")
	}
	after, _ := repo.Revision(context.Background())
	if userCount(t, repo, "rollback@example.com") != 0 || before != after {
		t.Fatal("partial user/revision escaped rollback")
	}
	if _, err := repo.DB().Exec("DROP TRIGGER fail_membership ON public.user_tenants"); err != nil {
		t.Fatal(err)
	}
	// The failed attempt warmed the true auto-add policy and negative user cache.
	tx, err := repo.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	off, _ := msgpack.Encode(false)
	if _, err = tx.Exec("UPDATE public.tenant_config SET value=$1 WHERE tenant_slug='alpha' AND key='auth:auto_add'", off); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		got, err := r.Resolve(context.Background(), input)
		if err == nil && got.Reject == nil {
			err = fmt.Errorf("enrolled against policy disabled while waiting")
		}
		done <- err
	}()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err = repo.DB().QueryRow("SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT revision FROM public.auth_admin_state%FOR UPDATE%')").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("enrollment did not wait for the policy writer")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if userCount(t, repo, "rollback@example.com") != 0 {
		t.Fatal("policy race created user")
	}
}

func TestAutoAddRechecksCachedEligibility(t *testing.T) {
	for _, change := range []string{"disabled user", "disabled tenant", "removed domain", "changed claims", "malformed flag"} {
		t.Run(change, func(t *testing.T) {
			c, repo, _ := setup(t)
			c.call("POST", "/tenants", tenantBody("alpha"), 200)
			c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
			c.call("PUT", "/tenants/alpha/auth", autoPolicy(true), 200)
			r, _ := resolver.New(resolver.Options{Repo: repo})
			input := autoInput("cached@example.com", "google", "alpha")
			input.Claims["hd"] = "wrong.example.org"
			checkAuto(t, r, input, false) // Warm candidate, policies and absent user.
			switch change {
			case "disabled user":
				c.call("POST", "/users", map[string]any{"email": "cached@example.com", "name": "Disabled", "enabled": false}, 200)
			case "disabled tenant":
				c.call("PUT", "/tenants/alpha", map[string]any{"name": "Alpha", "enabled": false}, 200)
			case "removed domain":
				c.call("DELETE", "/tenants/alpha/domains/example.com", nil, 200)
			case "changed claims":
				c.call("PUT", "/tenants/alpha/auth", map[string]any{"checks": map[string]any{"google": map[string]any{"hd": "different.example.org"}}}, 200)
			case "malformed flag":
				raw, _ := msgpack.Encode("true")
				if _, err := repo.DB().Exec("UPDATE public.tenant_config SET value=$1 WHERE key='auth:auto_add'", raw); err != nil {
					t.Fatal(err)
				}
			}
			input.Claims["hd"] = "example.com"
			checkAuto(t, r, input, false)
			var memberships, audits int
			if err := repo.DB().QueryRow("SELECT (SELECT count(*) FROM public.user_tenants), (SELECT count(*) FROM public.auth_admin_audit WHERE action='auto_add')").Scan(&memberships, &audits); err != nil || memberships != 0 || audits != 0 {
				t.Fatal("stale eligibility wrote membership/audit", memberships, audits, err)
			}
			expected := 0
			if change == "disabled user" {
				expected = 1
			}
			if userCount(t, repo, "cached@example.com") != expected {
				t.Fatal("stale eligibility wrote user")
			}
		})
	}
}
