// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"

	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

// tenantLookup is the narrow slice of the MT repository the external plane
// needs to enrich /checkz with the session user's tenant list. The concrete
// *mtdb.Repo satisfies it; declaring the interface here (rather than importing
// resolver) keeps the external package decoupled and avoids an import cycle.
type tenantLookup interface {
	GetUserWithTenants(ctx context.Context, email string) (*mtdb.User, error)
}

// checkzTenant is the per-tenant entry in the /checkz response. Field names
// match the SPA's tenant-selection contract exactly (id is the tenant slug;
// default_workspace is snake_case).
type checkzTenant struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Logo             string `json:"logo"`
	DefaultWorkspace string `json:"default_workspace"`
}

// handleCheckz answers the external session-status endpoint the SPA polls.
//
// Unlike the OSS handleCheckz (which returns only {auth,user_id,email,
// provider}), this one enriches the response with the session user's tenant
// list so the SPA can drive tenant selection. The tenant array mirrors
// resolver.arraysToTenants: slugs are the source of truth for length/ordering,
// and each parallel array (names/logos/default-workspace) is bounds-guarded so
// a shorter array yields an empty string rather than a panic.
//
// Graceful degradation: if the repo is nil or the lookup errors, the basic
// {auth,user_id,email} response is returned WITHOUT a tenants array (and
// without a 500) — a transient MT-DB hiccup must not break the session check.
func (s *Server) handleCheckz(w http.ResponseWriter, r *http.Request) {
	// Defense-in-depth Origin allowlist, enforced BEFORE any session lookup or
	// PII (email/user_id/tenants) processing. Applies to both /checkz and
	// /auth/checkz (both route here). Sits on top of the edge Envoy CORS policy.
	if !s.checkzOriginAllowed(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"origin not allowed"}`))
		return
	}

	data, err := s.lookupSession(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		log.Printf("external: checkz session lookup error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"session lookup failed"}`))
		return
	}
	if data == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"no session"}`))
		return
	}

	resp := map[string]any{
		"auth":    "valid",
		"user_id": data.UserID,
		"email":   data.Email,
	}

	if tenants, ok := s.lookupTenants(r.Context(), data.Email); ok {
		resp["tenants"] = tenants
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// lookupTenants resolves the session user's tenants via the MT repo and maps
// the parallel arrays into the response shape. The second return is false when
// no enrichment is possible (repo unset, lookup error, or unknown email) — the
// caller then omits the tenants array entirely (graceful degradation).
func (s *Server) lookupTenants(ctx context.Context, email string) ([]checkzTenant, bool) {
	if s.repo == nil || email == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	user, err := s.repo.GetUserWithTenants(ctx, email)
	if err != nil {
		log.Printf("external: checkz tenant lookup error for %q: %v", email, err)
		return nil, false
	}
	if user == nil {
		// Email not in MT — session is still valid, just no tenant list.
		return nil, false
	}

	// Mirror resolver.arraysToTenants: slugs drive length/ordering; the other
	// arrays are bounds-guarded so a shorter array yields "".
	n := len(user.TenantSlugs)
	out := make([]checkzTenant, 0, n)
	for i := 0; i < n; i++ {
		t := checkzTenant{ID: user.TenantSlugs[i]}
		if i < len(user.TenantNames) {
			t.Name = user.TenantNames[i]
		}
		if i < len(user.TenantLogos) {
			t.Logo = user.TenantLogos[i]
		}
		if i < len(user.TenantDefaultWS) {
			t.DefaultWorkspace = user.TenantDefaultWS[i]
		}
		out = append(out, t)
	}
	return out, true
}

// checkzOriginAllowed reports whether a /checkz (or /auth/checkz) request may
// proceed to session/PII processing, enforcing (1) a top-level-navigation block
// and (2) a strict server-side Origin allowlist, as defense-in-depth on top of
// the edge Envoy CORS policy.
//
// Decision table:
//   - Sec-Fetch-Mode: navigate (or Sec-Fetch-Dest: document) -> REJECT. /checkz
//     is a credentialed JSON/PII endpoint the SPA polls via fetch(), never a page
//     to navigate to. Browsers send these Fetch-Metadata headers ONLY on real
//     top-level navigations (typing the URL / clicking a link), never on fetch(),
//     so this kills "browse to /checkz and read your session JSON" without
//     affecting the SPA's cross-origin credentialed poll. Non-browser callers
//     (which omit Sec-Fetch-*) are unaffected by this rule.
//   - No Origin header (and not a navigation) -> ALLOW. Same-origin fetches and
//     non-browser callers omit Origin; the session cookie is still required, and
//     a cross-origin browser fetch (the exfiltration vector we care about) always
//     carries an Origin. So an absent Origin is not an attack signal here.
//   - Allowlist empty (unconfigured) -> ALLOW. Fail-open on config absence so
//     deployments that have not set SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS keep
//     working; the edge Envoy CORS policy is still in force.
//   - Origin present + allowlist non-empty -> ALLOW only on an exact,
//     case-sensitive match (scheme+host, as browsers send it). Any other Origin
//     is REJECTED (the caller returns 403 before touching the session).
func (s *Server) checkzOriginAllowed(r *http.Request) bool {
	// Block top-level browser navigations to this credentialed JSON/PII endpoint.
	if r.Header.Get("Sec-Fetch-Mode") == "navigate" || r.Header.Get("Sec-Fetch-Dest") == "document" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(s.opts.CheckzAllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range s.opts.CheckzAllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

// lookupSession returns the SessionData for the request, or (nil, nil) when no
// cookie is present or the session has expired/been revoked.
func (s *Server) lookupSession(r *http.Request) (*login.SessionData, error) {
	id := login.ReadSession(r, s.cookies)
	if id == "" {
		return nil, nil
	}
	return s.store.Get(r.Context(), id)
}
