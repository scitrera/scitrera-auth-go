// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package resolver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scitrera/scitrera-auth-go/internal/cache"
)

// cfgResolver builds a Resolver whose cfgCache is backed by a static map, so the
// per-tenant auth:providers allowlist logic can be exercised without a database.
func cfgResolver(vals map[string]any, errKeys map[string]bool) *Resolver {
	return &Resolver{
		cfgCache: cache.New[any](64, time.Minute, func(_ context.Context, key string) (any, error) {
			if errKeys[key] {
				return nil, errors.New("cfg lookup boom")
			}
			return vals[key], nil
		}),
	}
}

func TestProviderAllowed(t *testing.T) {
	const slug = "acme"
	key := slug + ":auth:providers"

	cases := []struct {
		name        string
		vals        map[string]any
		errKeys     map[string]bool
		provider    string
		wantAllowed bool
		wantLookup  bool
	}{
		{"unset allows any provider", map[string]any{}, nil, "azure", true, true},
		{"empty list allows any provider", map[string]any{key: []any{}}, nil, "azure", true, true},
		{"provider in allowlist passes", map[string]any{key: []any{"azure", "google"}}, nil, "azure", true, true},
		{"provider absent from allowlist drops", map[string]any{key: []any{"google"}}, nil, "azure", false, true},
		{"non-list value treated as no allowlist", map[string]any{key: "notalist"}, nil, "azure", true, true},
		{"lookup error fails closed", nil, map[string]bool{key: true}, "azure", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := cfgResolver(tc.vals, tc.errKeys)
			allowed, _, lookupOK := r.providerAllowed(context.Background(), slug, tc.provider)
			if allowed != tc.wantAllowed || lookupOK != tc.wantLookup {
				t.Fatalf("providerAllowed=%v lookupOK=%v; want allowed=%v lookupOK=%v",
					allowed, lookupOK, tc.wantAllowed, tc.wantLookup)
			}
		})
	}
}
