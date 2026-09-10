// SPDX-License-Identifier: AGPL-3.0-only
package resolver

import "testing"

func TestAutoAddRequiresOrganizationAuthority(t *testing.T) {
	const tid = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, provider    string
		providers, checks any
		claims            map[string]any
		allowed           bool
	}{
		{"workspace", "google", nil, map[string]any{"hd": []any{"example.com", ""}}, map[string]any{"hd": "example.com", "email_verified": true}, true},
		{"workspace other check required", "google", nil, map[string]any{"hd": "example.com", "department": "staff"}, map[string]any{"hd": "example.com", "email_verified": true}, false},
		{"blank is not authority", "google", nil, map[string]any{"hd": []any{""}}, map[string]any{"email_verified": true}, false},
		{"malformed allowlist", "google", "google", map[string]any{"hd": "example.com"}, map[string]any{"hd": "example.com", "email_verified": true}, false},
		{"malformed entry", "google", []any{"google", false}, map[string]any{"hd": "example.com"}, map[string]any{"hd": "example.com", "email_verified": true}, false},
		{"allowlist excludes provider", "google", []any{"azure"}, map[string]any{"hd": "example.com"}, map[string]any{"hd": "example.com", "email_verified": true}, false},
		{"azure", "azure", nil, map[string]any{"tid": tid}, map[string]any{"tid": tid}, true},
		{"entra alias", "entra", nil, map[string]any{"tid": []any{tid}}, map[string]any{"tid": tid}, true},
		{"microsoft alias", "microsoft", nil, map[string]any{"tid": tid}, map[string]any{"tid": tid}, true},
		{"organization must be constrained", "azure", nil, map[string]any{}, map[string]any{"tid": tid}, false},
		{"malformed organization", "azure", nil, map[string]any{"tid": "invalid"}, map[string]any{"tid": "invalid"}, false},
		{"personal Microsoft account", "azure", nil, map[string]any{"tid": "9188040D-6C67-4C5B-B112-36A304B66DAD"}, map[string]any{"tid": "9188040D-6C67-4C5B-B112-36A304B66DAD"}, false},
		{"generic providers require explicit support", "other", nil, map[string]any{"hd": "example.com"}, map[string]any{"hd": "example.com", "email_verified": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoAddAllowed(tc.provider, tc.providers, tc.checks, tc.claims); got != tc.allowed {
				t.Fatalf("allowed=%v want %v", got, tc.allowed)
			}
		})
	}
}
