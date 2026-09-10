// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	pkgauthproxy "github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"

	"github.com/scitrera/scitrera-auth-go/internal/cache"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

// testResolver builds a Resolver whose caches are backed by fixtures instead of
// Postgres, so Resolve() can be exercised end-to-end without a database. The
// loaders never touch r.repo, so leaving it nil is safe.
func testResolver(user *mtdb.User) *Resolver {
	r := &Resolver{log: zerolog.Nop()}
	r.userCache = cache.New[*mtdb.User](8, time.Minute,
		func(context.Context, string) (*mtdb.User, error) { return user, nil })
	r.domCache = cache.New[*mtdb.Tenant](8, time.Minute,
		func(context.Context, string) (*mtdb.Tenant, error) { return nil, nil })
	r.cfgCache = cache.New[any](8, time.Minute,
		func(context.Context, string) (any, error) { return nil, nil })
	return r
}

// memberOf builds an enabled MT user belonging to the given tenant slugs, with
// the FIRST slug as their MT default tenant.
func memberOf(slugs ...string) *mtdb.User {
	u := &mtdb.User{
		ID: "u@example.com", Email: "u@example.com", Name: "U", Enabled: true,
		TenantSlugs: slugs,
	}
	if len(slugs) > 0 {
		u.DefaultTenantSlug = slugs[0]
	}
	return u
}

// checkRequest mimics the ext_authz check request Envoy sends: the ORIGINAL
// path is gone (replaced by pathOverride) and the tenant arrives only as the
// server-set query parameter.
func checkRequest(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/auth/verify"+query, nil)
}

func resolveWith(t *testing.T, user *mtdb.User, query string) *pkgauthproxy.ResolvedIdentity {
	t.Helper()
	got, err := testResolver(user).Resolve(context.Background(), pkgauthproxy.ResolverInput{
		Identity: models.Identity{ID: "u@example.com", Type: models.PrincipalUser},
		Method:   "session",
		Claims:   map[string]any{"email": "u@example.com"},
		Request:  checkRequest(query),
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got == nil {
		t.Fatal("Resolve returned nil identity")
	}
	return got
}

// A member of the requested tenant is admitted.
func TestResolve_RequestedTenant_MemberAdmitted(t *testing.T) {
	got := resolveWith(t, memberOf("acme"), "?workspace_id=_tenant_edge&tenant_id=acme")

	if got.Reject != nil {
		t.Fatalf("member was rejected: %+v", got.Reject)
	}
	if got.DefaultTenantID != "acme" {
		t.Errorf("DefaultTenantID = %q, want acme", got.DefaultTenantID)
	}
}

// A non-member is refused at the edge — this is the cross-tenant gate. Without
// it, the permissive _tenant_edge ACL row would let any authenticated user
// reach any tenant's route, since the ACL check has no tenant dimension.
func TestResolve_RequestedTenant_NonMemberRejected(t *testing.T) {
	got := resolveWith(t, memberOf("acme"), "?workspace_id=_tenant_edge&tenant_id=globex")

	if got.Reject == nil {
		t.Fatal("non-member was ADMITTED to a foreign tenant")
	}
	if got.Reject.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", got.Reject.Status)
	}
	if got.Reject.Code != "tenant_not_permitted" {
		t.Errorf("code = %q, want tenant_not_permitted", got.Reject.Code)
	}
}

// A multi-tenant user is pinned to the tenant they actually addressed, not to
// their MT default. Previously the default was injected as X-Auth-Tenant-ID
// regardless of which tenant's route the request arrived on.
func TestResolve_RequestedTenant_PinsIdentityToAddressedTenant(t *testing.T) {
	user := memberOf("acme", "globex") // MT default is acme
	got := resolveWith(t, user, "?workspace_id=_tenant_edge&tenant_id=globex")

	if got.Reject != nil {
		t.Fatalf("member of both tenants was rejected: %+v", got.Reject)
	}
	if got.DefaultTenantID != "globex" {
		t.Errorf("DefaultTenantID = %q, want globex (the addressed tenant, not the MT default)", got.DefaultTenantID)
	}
	if got.ExtraHeaders[headerScitreraDefaultTenant] != "globex" {
		t.Errorf("%s = %q, want globex", headerScitreraDefaultTenant,
			got.ExtraHeaders[headerScitreraDefaultTenant])
	}
}

// Non-tenant-scoped gates (superadmin, mlflow, blobgw) set no tenant_id and
// must keep behaving exactly as before.
func TestResolve_NoTenantParam_SkipsGate(t *testing.T) {
	got := resolveWith(t, memberOf("acme"), "?workspace_id=_superadmin")

	if got.Reject != nil {
		t.Fatalf("non-tenant-scoped gate was rejected: %+v", got.Reject)
	}
	if got.DefaultTenantID != "acme" {
		t.Errorf("DefaultTenantID = %q, want the MT default (acme)", got.DefaultTenantID)
	}
}

// Slug comparison is case/space tolerant so a stray pathOverride edit does not
// silently lock a tenant out.
func TestResolve_RequestedTenant_CaseAndSpaceTolerant(t *testing.T) {
	got := resolveWith(t, memberOf("acme"), "?tenant_id=%20ACME%20")
	if got.Reject != nil {
		t.Fatalf("case/space variant rejected: %+v", got.Reject)
	}
}

// A disabled MT user is refused regardless of tenant — MT remains able to veto
// access no matter what the per-tenant plane says.
func TestResolve_DisabledUser_RejectedEvenWithValidTenant(t *testing.T) {
	user := memberOf("acme")
	user.Enabled = false

	got := resolveWith(t, user, "?workspace_id=_tenant_edge&tenant_id=acme")
	if got.Reject == nil {
		t.Fatal("disabled user was admitted")
	}
	if got.Reject.Status != http.StatusForbidden || got.Reject.Code != "user_disabled" {
		t.Errorf("reject = %d/%s, want 403/user_disabled", got.Reject.Status, got.Reject.Code)
	}
}

// Machine principals (api_key / task_token) bypass MT resolution entirely, so
// the tenant gate must not apply to them.
func TestResolve_MachinePrincipal_BypassesTenantGate(t *testing.T) {
	got, err := testResolver(memberOf("acme")).Resolve(context.Background(), pkgauthproxy.ResolverInput{
		Identity: models.Identity{ID: "svc::worker", Type: models.PrincipalService},
		Method:   "api_key",
		Request:  checkRequest("?workspace_id=_tenant_edge&tenant_id=globex"),
	})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if got.Reject != nil {
		t.Fatalf("machine principal rejected by the tenant gate: %+v", got.Reject)
	}
}

func TestRequestedTenant_Parsing(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{"absent", "?workspace_id=_superadmin", ""},
		{"present", "?tenant_id=acme", "acme"},
		{"with workspace", "?workspace_id=_tenant_edge&tenant_id=acme", "acme"},
		{"empty value", "?tenant_id=", ""},
		{"normalized", "?tenant_id=%20AcMe%20", "acme"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requestedTenant(checkRequest(c.query)); got != c.want {
				t.Errorf("requestedTenant(%q) = %q, want %q", c.query, got, c.want)
			}
		})
	}
	if got := requestedTenant(nil); got != "" {
		t.Errorf("requestedTenant(nil) = %q, want empty", got)
	}
}
