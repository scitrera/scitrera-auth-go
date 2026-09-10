// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListenerIsolationAndSharing(t *testing.T) {
	named := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(name)) })
	}
	for _, tc := range []struct {
		name, external, admin, metrics, expectMetrics string
		extras                                        int
	}{
		{"separate defaults", ":8081", ":8082", "", ":8082", 2},
		{"dedicated metrics", ":8081", ":8082", ":9090", ":9090", 3},
		{"admin absent", ":8081", "", "", ":8080", 1},
		{"admin shares internal", ":8081", ":8080", "", ":8080", 1},
		{"all explicitly share", ":8080", ":8080", "", ":8080", 0},
		{"admin shares public", ":8081", ":8081", "", ":8081", 1},
		{"metrics disabled", ":8081", ":8082", "off", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := planListeners(":8080", tc.external, tc.admin, tc.metrics)
			if err != nil {
				t.Fatal(err)
			}
			if p.metrics != tc.expectMetrics || len(p.extraAddresses()) != tc.extras {
				t.Fatalf("unexpected plan %+v", p)
			}
			for _, addr := range append([]string{p.internal}, p.extraAddresses()...) {
				h := p.handler(addr, named("internal"), named("external"), named("admin"), named("metrics"))
				for _, path := range []string{"/metrics", "/admin/tenants/alpha", "/api/auth-admin/v1/users", "/auth/login/google", "/auth/verify", "/healthz"} {
					req := httptest.NewRequest("GET", path, nil)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, req)
					switch {
					case path == "/metrics":
						if addr == p.metrics {
							if w.Body.String() != "metrics" {
								t.Fatal("metrics missing", addr)
							}
						} else if w.Code != 404 {
							t.Fatal("metrics escaped its listener", addr)
						}
					case path == "/admin/tenants/alpha" || path == "/api/auth-admin/v1/users":
						if (w.Body.String() == "admin") != (addr == p.admin) {
							t.Fatal("admin listener boundary", addr, path, w.Body.String())
						}
					case path == "/auth/login/google":
						if addr == p.external && w.Body.String() != "external" {
							t.Fatal("public login missing")
						}
						if addr != p.internal && addr != p.external && w.Code != 404 {
							t.Fatal("login exposed on private-only listener")
						}
					case path == "/auth/verify":
						if (w.Body.String() == "internal") != (addr == p.internal) {
							t.Fatal("verification listener boundary", addr)
						}
					}
				}
			}
		})
	}
	if _, err := planListeners(":8080", "", ":8082", "not-an-address"); err == nil {
		t.Fatal("invalid metrics address accepted")
	}
}
