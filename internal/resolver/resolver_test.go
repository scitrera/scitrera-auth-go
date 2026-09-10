// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package resolver

import (
	"testing"
)

func TestEvaluateChecks_AzureTID_List(t *testing.T) {
	checks := map[string]any{
		"tid": []any{"11111111-1111-4111-8111-111111111111", "other-tid"},
	}

	t.Run("matching tid passes", func(t *testing.T) {
		ok, _ := evaluateChecks(checks, map[string]any{
			"tid": "11111111-1111-4111-8111-111111111111",
		})
		if !ok {
			t.Fatal("expected pass")
		}
	})

	t.Run("foreign tid rejected", func(t *testing.T) {
		ok, _ := evaluateChecks(checks, map[string]any{
			"tid": "00000000-0000-0000-0000-000000000000",
		})
		if ok {
			t.Fatal("expected reject")
		}
	})

	t.Run("missing tid rejected", func(t *testing.T) {
		ok, _ := evaluateChecks(checks, map[string]any{})
		if ok {
			t.Fatal("expected reject for missing claim")
		}
	})
}

func TestEvaluateChecks_StringEquality(t *testing.T) {
	checks := map[string]any{"hd": "example.com"}

	if ok, _ := evaluateChecks(checks, map[string]any{"hd": "example.com"}); !ok {
		t.Fatal("expected pass for matching scalar string")
	}
	if ok, _ := evaluateChecks(checks, map[string]any{"hd": "evil.com"}); ok {
		t.Fatal("expected reject for non-matching scalar string")
	}
}

func TestEvaluateChecks_MultipleClaimsAllRequired(t *testing.T) {
	// Multi-claim auth_checks: ALL must satisfy.
	checks := map[string]any{
		"tid": []any{"expected-tid"},
		"hd":  "example.com",
	}
	t.Run("both pass", func(t *testing.T) {
		ok, _ := evaluateChecks(checks, map[string]any{
			"tid": "expected-tid", "hd": "example.com",
		})
		if !ok {
			t.Fatal("expected pass")
		}
	})
	t.Run("one fail rejects", func(t *testing.T) {
		ok, _ := evaluateChecks(checks, map[string]any{
			"tid": "expected-tid", "hd": "evil.com",
		})
		if ok {
			t.Fatal("expected reject when any single claim fails")
		}
	})
}

func TestArraysToTenants_PaddingMismatchedLengths(t *testing.T) {
	// Real MT data sometimes has nil entries inside arrays; the resolver
	// must not panic on uneven lengths.
	got := arraysToTenants(
		[]string{"a", "b"},
		[]string{"Alpha"}, // missing entry for "b"
		[]string{"logo-a", "logo-b"},
		[]string{"ws-a"},
	)
	if len(got) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(got))
	}
	if got[0].Slug != "a" || got[0].Name != "Alpha" {
		t.Errorf("tenant[0]: got %+v", got[0])
	}
	if got[1].Slug != "b" || got[1].Name != "" {
		t.Errorf("tenant[1]: got %+v", got[1])
	}
	if got[1].Logo != "logo-b" {
		t.Errorf("tenant[1].Logo: got %q", got[1].Logo)
	}
	if got[1].DefaultWS != "" {
		t.Errorf("tenant[1].DefaultWS: got %q", got[1].DefaultWS)
	}
}

func TestEmailDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alice@example.com", "example.com"},
		{"BOB@EXAMPLE.COM", "example.com"},
		{"no-at", ""},
		{"", ""},
		{"weird@host@nested", "nested"},
	}
	for _, tc := range cases {
		if got := emailDomain(tc.in); got != tc.want {
			t.Errorf("emailDomain(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEmailFromClaims_FallbackOrder(t *testing.T) {
	cases := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{"email present", map[string]any{"email": "e@x.com", "upn": "u@x.com"}, "e@x.com"},
		{"upn fallback", map[string]any{"upn": "u@x.com"}, "u@x.com"},
		{"preferred_username fallback", map[string]any{"preferred_username": "p@x.com"}, "p@x.com"},
		{"none → identity id", map[string]any{}, "identity-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailFromClaims(tc.claims, "identity-id"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsUserMethod(t *testing.T) {
	for _, m := range []string{"oauth", "azure_entra", "session"} {
		if !isUserMethod(m) {
			t.Errorf("%q should be a user method", m)
		}
	}
	for _, m := range []string{"api_key", "task_token", ""} {
		if isUserMethod(m) {
			t.Errorf("%q should NOT be a user method", m)
		}
	}
}
