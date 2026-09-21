// SPDX-License-Identifier: AGPL-3.0-only
package admin_test

import (
	"context"
	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/resolver"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMembershipChecksLifecycleAndAutoAddIsolation(t *testing.T) {
	c, repo, _ := setup(t)
	c.call("POST", "/tenants", tenantBody("alpha"), 200)
	c.call("POST", "/tenants", tenantBody("beta"), 200)
	c.call("POST", "/users", userBody(true), 200)
	id := c.call("GET", "/users", nil, 200)["data"].([]any)[0].(map[string]any)["id"].(string)
	membership := "/users/" + id + "/memberships"
	path := membership + "/alpha/auth"
	override := map[string]any{"checks": map[string]any{"azure": map[string]any{"tid": []string{"22222222-2222-4222-8222-222222222222"}}}}
	c.call("GET", path, nil, 404)
	c.call("PUT", path, override, 404) // Must never create a membership.
	for _, slug := range []string{"alpha", "beta"} {
		c.call("POST", membership, map[string]string{"tenant_slug": slug}, 200)
		c.call("PUT", "/tenants/"+slug+"/auth", map[string]any{"providers": []string{"azure"}, "checks": map[string]any{"azure": map[string]any{"tid": "11111111-1111-4111-8111-111111111111", "department": "research"}}}, 200)
	}
	r1, _ := resolver.New(resolver.Options{Repo: repo})
	r2, _ := resolver.New(resolver.Options{Repo: repo})
	check := func(r *resolver.Resolver, tenant, tid, department string, allowed bool) {
		t.Helper()
		got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: "person@example.com", Type: models.PrincipalUser}, Claims: map[string]any{"email": "person@example.com", "provider": "azure", "tid": tid, "department": department}, Request: httptest.NewRequest("GET", "/auth/verify?tenant_id="+tenant, nil)})
		if err != nil {
			t.Fatal(err)
		}
		if (got.Reject == nil) != allowed {
			t.Fatalf("%s tid=%s allowed=%v want=%v reject=%+v", tenant, tid, got.Reject == nil, allowed, got.Reject)
		}
	}
	base, alt := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	for _, r := range []*resolver.Resolver{r1, r2} {
		check(r, "alpha", alt, "research", false)
	}
	for _, bad := range []any{map[string]any{}, map[string]any{"checks": nil}, map[string]any{"checks": map[string]any{"azure": nil}}, map[string]any{"checks": map[string]any{"azure": map[string]any{"tid": nil}}}, map[string]any{"checks": map[string]any{"azure": map[string]any{"tid": ""}}}, map[string]any{"checks": map[string]any{}, "providers": []string{"google"}}} {
		c.call("PUT", path, bad, 400)
	}
	c.call("PUT", path, override, 200)
	stored := c.call("GET", path, nil, 200)["data"].(map[string]any)["checks"].(map[string]any)
	if stored["azure"].(map[string]any)["tid"].([]any)[0] != alt {
		t.Fatal(stored)
	}
	time.Sleep(1100 * time.Millisecond)
	for _, r := range []*resolver.Resolver{r1, r2} {
		check(r, "alpha", alt, "research", true)
		check(r, "alpha", alt, "wrong", false)
		check(r, "alpha", base, "research", false)
		check(r, "beta", alt, "research", false)
	}
	// Explicit overrides never bypass a disabled user or tenant.
	for _, target := range []string{"user", "tenant"} {
		if target == "user" {
			c.call("PUT", "/users/"+id, userBody(false), 200)
		} else {
			body := tenantBody("alpha")
			delete(body, "slug")
			body["enabled"] = false
			c.call("PUT", "/tenants/alpha", body, 200)
		}
		fresh, _ := resolver.New(resolver.Options{Repo: repo})
		check(fresh, "alpha", alt, "research", false)
		if target == "user" {
			c.call("PUT", "/users/"+id, userBody(true), 200)
		} else {
			body := tenantBody("alpha")
			delete(body, "slug")
			c.call("PUT", "/tenants/alpha", body, 200)
		}
	}
	// Removing the override must propagate to already-warm replicas.
	c.call("PUT", path, map[string]any{"checks": map[string]any{}}, 200)
	time.Sleep(1100 * time.Millisecond)
	for _, r := range []*resolver.Resolver{r1, r2} {
		check(r, "alpha", alt, "research", false)
		check(r, "alpha", base, "research", true)
	}
	c.call("PUT", path, override, 200)
	// Removing/recreating membership must not resurrect an override, even via auto-add.
	c.call("POST", "/tenants/alpha/domains", map[string]string{"domain": "example.com"}, 200)
	c.call("PUT", "/tenants/alpha/auth", map[string]any{"auto_add": true}, 200)
	c.call("DELETE", membership+"/alpha", nil, 200)
	time.Sleep(1100 * time.Millisecond)
	check(r1, "alpha", alt, "research", false)
	c.call("GET", path, nil, 404)
	check(r1, "alpha", base, "research", true)
	empty := c.call("GET", path, nil, 200)["data"].(map[string]any)["checks"].(map[string]any)
	if len(empty) != 0 {
		t.Fatal("auto-add resurrected overrides", empty)
	}
	check(r2, "alpha", alt, "research", false)
	// Both membership edits and the new API use the existing revision/audit path.
	var edits int
	if err := repo.DB().QueryRow(`SELECT count(*) FROM public.auth_admin_audit WHERE resource=$1 AND fields=ARRAY['membership:auth_checks']`, path[1:]).Scan(&edits); err != nil || edits != 3 {
		t.Fatal("override audit", edits, err)
	}
}
