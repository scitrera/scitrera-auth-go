// SPDX-License-Identifier: AGPL-3.0-only
package resolver

import (
	"context"
	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/cache"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
	"testing"
	"time"
)

func TestDomainAssociationRequiresExplicitAutoAdd(t *testing.T) {
	for _, tc := range []struct {
		name          string
		user          *mtdb.User
		domainEnabled bool
		provider      string
		allowed       bool
	}{
		{"unknown matching domain without auto-add", nil, true, "azure", false},
		{"unknown disabled domain", nil, false, "azure", false},
		{"unknown wrong provider", nil, true, "google", false},
		{"known disabled user", &mtdb.User{Enabled: false}, true, "azure", false},
		{"known without membership", &mtdb.User{Enabled: true}, true, "azure", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testResolver(tc.user)
			r.domCache = cache.New(8, time.Minute, func(context.Context, string) (*mtdb.Tenant, error) {
				return &mtdb.Tenant{Slug: "example", Enabled: tc.domainEnabled}, nil
			})
			r.cfgCache = cache.New[any](8, time.Minute, func(_ context.Context, key string) (any, error) {
				if key == "example:auth:providers" {
					return []any{"azure"}, nil
				}
				return nil, nil
			})
			got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: "unknown@example.com", Type: models.PrincipalUser}, Claims: map[string]any{"email": "unknown@example.com", "provider": tc.provider}, Request: checkRequest("?tenant_id=example")})
			if err != nil {
				t.Fatal(err)
			}
			if (got.Reject == nil) != tc.allowed {
				t.Fatalf("admission=%v want %v: %+v", got.Reject == nil, tc.allowed, got)
			}
		})
	}
}
