// SPDX-License-Identifier: AGPL-3.0-only
package resolver

import (
	"context"
	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/cache"
	"testing"
	"time"
)

func TestMembershipClaimOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, tenant, provider, tid, department string
		member, enabled, override, allow        bool
	}{
		{"exact membership", "example", "azure", "alternate", "research", true, true, true, true},
		{"other tenant", "other", "azure", "alternate", "research", true, true, true, false},
		{"wrong provider", "example", "google", "alternate", "research", true, true, true, false},
		{"wrong organization", "example", "azure", "wrong", "research", true, true, true, false},
		{"other claim still required", "example", "azure", "alternate", "wrong", true, true, true, false},
		{"disabled user", "example", "azure", "alternate", "research", true, false, true, false},
		{"missing membership", "example", "azure", "alternate", "research", false, true, true, false},
		{"ordinary member cannot use alternate", "example", "azure", "alternate", "research", true, true, false, false},
		{"ordinary member keeps base rule", "example", "azure", "base", "research", true, true, false, true},
		{"override replaces rather than adds", "example", "azure", "base", "research", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := memberOf("example", "other")
			user.Enabled = tc.enabled
			if !tc.member {
				user = memberOf()
			}
			if tc.override {
				user.MembershipChecks = map[string]map[string]map[string]any{"example": {"azure": {"tid": []any{"alternate"}}}}
			}
			r := testResolver(user)
			base := map[string]any{"tid": "base", "department": "research"}
			r.cfgCache = cache.New[any](8, time.Minute, func(_ context.Context, key string) (any, error) {
				switch key {
				case "example:auth:providers", "other:auth:providers":
					return []any{"azure"}, nil
				case "example:auth:checks:azure", "other:auth:checks:azure":
					return base, nil
				}
				return nil, nil
			})
			got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: "u@example.com", Type: models.PrincipalUser}, Claims: map[string]any{"email": "u@example.com", "provider": tc.provider, "tid": tc.tid, "department": tc.department}, Request: checkRequest("?tenant_id=" + tc.tenant)})
			if err != nil {
				t.Fatal(err)
			}
			if (got.Reject == nil) != tc.allow {
				t.Fatalf("allowed=%v want %v: %+v", got.Reject == nil, tc.allow, got.Reject)
			}
			if base["tid"] != "base" {
				t.Fatal("shared tenant policy was mutated")
			}
		})
	}
}

func TestMalformedMembershipClaimOverridesFailClosed(t *testing.T) {
	for _, overrides := range []map[string]any{nil, {}, {"tid": nil}, {"tid": true}, {"tid": []any{}}} {
		u := memberOf("example")
		u.MembershipChecks = map[string]map[string]map[string]any{"example": {"azure": overrides}}
		r := testResolver(u)
		got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: "u@example.com", Type: models.PrincipalUser}, Claims: map[string]any{"email": "u@example.com", "provider": "azure", "tid": "base"}, Request: checkRequest("?tenant_id=example")})
		if err != nil {
			t.Fatal(err)
		}
		if got.Reject == nil {
			t.Fatalf("malformed override admitted: %#v", overrides)
		}
	}
}
