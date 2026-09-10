// SPDX-License-Identifier: AGPL-3.0-only
package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/models"
	"github.com/scitrera/scitrera-auth-go/internal/cache"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

func TestGoogleHostedDomainOptions(t *testing.T) {
	domains := []any{"example.com", "second.example.org"}
	guests := []any{"example.com", "second.example.org", ""}
	disabled := memberOf("example")
	disabled.Enabled = false
	for _, tc := range []struct {
		name       string
		user       *mtdb.User
		rule       any
		hd         any
		present    bool
		provider   string
		extraCheck bool
		allowed    bool
	}{
		{"member first domain", memberOf("example"), domains, "example.com", true, "google", false, true},
		{"member second domain", memberOf("example"), domains, "second.example.org", true, "google", false, true},
		{"wrong domain is not blank", memberOf("example"), guests, "other.example.org", true, "google", false, false},
		{"member missing hd explicitly allowed", memberOf("example"), guests, nil, false, "google", false, true},
		{"member empty hd explicitly allowed", memberOf("example"), guests, "", true, "google", false, true},
		{"member missing hd not allowed", memberOf("example"), domains, nil, false, "google", false, false},
		{"null hd is malformed", memberOf("example"), guests, nil, true, "google", false, false},
		{"boolean hd is malformed", memberOf("example"), guests, true, true, "google", false, false},
		{"domain-only missing hd denied", nil, guests, nil, false, "google", false, false},
		{"domain-only empty hd denied", nil, guests, "", true, "google", false, false},
		{"auto-add off first authoritative domain", nil, guests, "example.com", true, "google", false, false},
		{"auto-add off second authoritative domain", nil, guests, "second.example.org", true, "google", false, false},
		{"domain-only wrong domain", nil, guests, "other.example.org", true, "google", false, false},
		{"disabled member cannot use blank", disabled, guests, nil, false, "google", false, false},
		{"user without membership cannot use blank", memberOf(), guests, nil, false, "google", false, false},
		{"membership of another tenant cannot use blank", memberOf("other"), guests, nil, false, "google", false, false},
		{"empty option list denies member", memberOf("example"), []any{}, nil, false, "google", false, false},
		{"legacy scalar domain", memberOf("example"), "example.com", "example.com", true, "google", false, true},
		{"scalar blank permits member", memberOf("example"), "", nil, false, "google", false, true},
		{"scalar blank denies domain-only", nil, "", nil, false, "google", false, false},
		{"other provider cannot use missing claim exception", memberOf("example"), guests, nil, false, "azure", false, false},
		{"blank does not bypass another claim", memberOf("example"), guests, nil, false, "google", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testResolver(tc.user)
			domainLookups := 0
			r.domCache = cache.New(8, time.Minute, func(context.Context, string) (*mtdb.Tenant, error) {
				domainLookups++
				return &mtdb.Tenant{Slug: "example", Enabled: true}, nil
			})
			r.cfgCache = cache.New[any](8, time.Minute, func(_ context.Context, key string) (any, error) {
				if key == "example:auth:checks:"+tc.provider || key == "other:auth:checks:"+tc.provider {
					checks := map[string]any{"hd": tc.rule}
					if tc.extraCheck {
						checks["department"] = "research"
					}
					return checks, nil
				}
				return nil, nil
			})
			claims := map[string]any{"email": "u@example.com", "provider": tc.provider}
			if tc.present {
				claims["hd"] = tc.hd
			}
			got, err := r.Resolve(context.Background(), authproxy.ResolverInput{Method: "session", Identity: models.Identity{ID: "u@example.com", Type: models.PrincipalUser}, Claims: claims, Request: checkRequest("?tenant_id=example")})
			if err != nil {
				t.Fatal(err)
			}
			if (got.Reject == nil) != tc.allowed {
				t.Fatalf("allowed=%v want %v: %+v", got.Reject == nil, tc.allowed, got.Reject)
			}
			if tc.user != nil && domainLookups != 0 {
				t.Fatal("registered user fell back to domain admission")
			}
			if _, present := claims["hd"]; present != tc.present {
				t.Fatal("verified claims were mutated")
			}
		})
	}
}
