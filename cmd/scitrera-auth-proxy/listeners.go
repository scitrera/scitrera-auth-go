// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Identical configured addresses explicitly share one listener. Default metrics
// follow the admin address, then the trusted internal address if admin is off.
type listenerPlan struct{ internal, external, admin, metrics string }

func planListeners(internal, external, admin, metrics string) (listenerPlan, error) {
	p := listenerPlan{internal, external, admin, metrics}
	if metrics == "off" {
		p.metrics = ""
	} else if metrics == "" {
		p.metrics = admin
		if p.metrics == "" {
			p.metrics = internal
		}
	}
	for name, addr := range map[string]string{"internal": p.internal, "external": p.external, "admin": p.admin, "metrics": p.metrics} {
		if addr == "" {
			continue
		}
		_, port, err := net.SplitHostPort(addr)
		if err != nil || port == "" {
			return p, fmt.Errorf("invalid %s listen address %q: use host:port", name, addr)
		}
	}
	return p, nil
}
func (p listenerPlan) extraAddresses() []string {
	seen := map[string]bool{p.internal: true, "": true}
	var out []string
	for _, addr := range []string{p.external, p.admin, p.metrics} {
		if !seen[addr] {
			seen[addr] = true
			out = append(out, addr)
		}
	}
	return out
}
func (p listenerPlan) handler(addr string, internal, external, admin, metrics http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			if addr == p.metrics && metrics != nil {
				if r.Method != "GET" && r.Method != "HEAD" {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				metrics.ServeHTTP(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		adminPath := r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") || r.URL.Path == "/api/auth-admin/v1" || strings.HasPrefix(r.URL.Path, "/api/auth-admin/v1/")
		if adminPath && addr == p.admin && admin != nil {
			if r.URL.Path == "/admin" {
				http.Redirect(w, r, "/admin/", http.StatusSeeOther)
				return
			}
			admin.ServeHTTP(w, r)
			return
		}
		// Browser endpoints take precedence when explicitly sharing the internal listener.
		browserPath := r.URL.Path == "/" || r.URL.Path == "/login" || r.URL.Path == "/checkz" || r.URL.Path == "/source.tar.gz" || r.URL.Path == "/auth/logout" || r.URL.Path == "/auth/checkz" || strings.HasPrefix(r.URL.Path, "/auth/login/") || strings.HasPrefix(r.URL.Path, "/auth/callback/")
		if addr == p.external && external != nil && (addr != p.internal || browserPath) {
			external.ServeHTTP(w, r)
			return
		}
		if addr == p.internal && internal != nil {
			internal.ServeHTTP(w, r)
			return
		}
		if addr == p.admin && admin != nil && r.URL.Path == "/" {
			admin.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" && (r.Method == "GET" || r.Method == "HEAD") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
}
