// SPDX-License-Identifier: AGPL-3.0-only
package integration_test

import (
	"net/http"
	"net/url"
	"testing"
)

const microsoftHost = "https://login.microsoftonline.com"
const microsoftAuthority = microsoftHost + "/organizations/v2.0"

type organizationAccount struct {
	tid, email string
	allow      bool
}

// Exercise the production provider factory on both public and internal listeners,
// with real OAuth redirects, signed tokens, sessions and PostgreSQL admission.
func TestMicrosoftOrganizationsLoginAndAutoAdd(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account organizationAccount
	}{
		{"enroll eligible organization with uppercase domain", organizationAccount{"11111111-1111-4111-8111-111111111111", "person@EXAMPLE.COM", true}},
		{"deny another organization despite matching email domain", organizationAccount{"22222222-2222-4222-8222-222222222222", "person@example.com", false}},
		{"deny another domain despite matching organization", organizationAccount{"11111111-1111-4111-8111-111111111111", "person@other.example", false}},
	} {
		t.Run(tc.name, func(t *testing.T) { syntheticOIDC(t, true, false, &tc.account) })
	}
}

type microsoftTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (m microsoftTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "login.microsoftonline.com" {
		return m.base.RoundTrip(r)
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = m.target.Scheme, m.target.Host
	clone.URL, clone.Host = &u, u.Host
	return m.base.RoundTrip(clone)
}
func mockMicrosoftTransport(t *testing.T, target string) {
	t.Helper()
	// These tests are deliberately serial, including cleanup of every listener
	// before restoring the default transport. No real Microsoft traffic is sent.
	original := http.DefaultTransport
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	http.DefaultTransport = microsoftTransport{base: original, target: u}
	t.Cleanup(func() { http.DefaultTransport = original })
}
