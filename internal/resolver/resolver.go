// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package resolver implements ScitreraIdentityResolver — a multi-tenant
// IdentityResolver that ports
// backend/scitrera_forward_auth/main.py:_auth_common into Go.
//
// Existing enabled memberships are checked against tenant provider policies.
// A missing membership requires explicit tenant auto-add, an associated email
// domain and authoritative organization claims. Enrollment persists a user and
// membership atomically; an associated domain alone never grants access.
package resolver

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"

	pkgauthproxy "github.com/scitrera/aether/server/pkg/authproxy"

	"github.com/scitrera/scitrera-auth-go/internal/cache"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

// Options configures the resolver. The Repo and Logger are required.
type Options struct {
	Repo   *mtdb.Repo
	Logger zerolog.Logger
	// CacheTTL is the TTL for hot-path lookups (mt user, tenant-by-domain,
	// auth_checks). Defaults to 5 minutes (mirrors the Python helpers).
	CacheTTL time.Duration
	// CacheCapacity bounds the in-memory cache. Defaults to 16384.
	CacheCapacity int
}

// Resolver implements pkgauthproxy.IdentityResolver against the MT Postgres
// schema. Safe for concurrent use; internal caches handle stampede protection.
type Resolver struct {
	repo         *mtdb.Repo
	log          zerolog.Logger
	userCache    *cache.TTLCache[*mtdb.User]
	domCache     *cache.TTLCache[*mtdb.Tenant]
	revisionGate *cache.RevisionGate
	cfgCache     *cache.TTLCache[any] // keyed by "<tenant>:<configKey>"
}

// New constructs a Resolver. Cache size/TTL default to 16384 / 5min when zero.
func New(opts Options) (*Resolver, error) {
	if opts.Repo == nil {
		return nil, fmt.Errorf("resolver: Repo is required")
	}
	ttl := opts.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	cap_ := opts.CacheCapacity
	if cap_ == 0 {
		cap_ = 16384
	}

	r := &Resolver{repo: opts.Repo, log: opts.Logger}
	r.userCache = cache.New[*mtdb.User](cap_, ttl, func(ctx context.Context, email string) (*mtdb.User, error) {
		return r.repo.GetUserWithTenants(ctx, email)
	})
	r.domCache = cache.New[*mtdb.Tenant](cap_, ttl, func(ctx context.Context, domain string) (*mtdb.Tenant, error) {
		return r.repo.GetTenantByDomain(ctx, domain)
	})
	r.cfgCache = cache.New[any](cap_, ttl, func(ctx context.Context, key string) (any, error) {
		i := strings.IndexByte(key, ':')
		if i < 0 {
			return nil, fmt.Errorf("invalid cfg cache key %q", key)
		}
		return r.repo.GetTenantConfigParam(ctx, key[:i], key[i+1:])
	})
	r.revisionGate = &cache.RevisionGate{Interval: time.Second, Load: r.repo.Revision, Invalidate: func() { r.userCache.Purge(); r.domCache.Purge(); r.cfgCache.Purge() }}
	return r, nil
}

// requestedTenantParam is the query parameter a TENANT-SCOPED gate sets on the
// ext_authz check request, naming the tenant the client is trying to reach.
//
// WHY a query parameter: the check request Envoy sends carries no tenant
// identity of its own. `pathOverride` REPLACES the original path, so the
// "/{tenant}/..." prefix — the only place the target tenant appears — is gone
// before auth-go sees the request, and `headersToExtAuth` forwards just cookie
// and authorization. Putting the tenant on the SecurityPolicy's pathOverride
// (".../auth/verify?workspace_id=<class>&tenant_id=<slug>") is what makes a
// per-tenant decision possible at all.
//
// SAFETY: the value is SERVER-set and cannot be spoofed. It comes from the
// SecurityPolicy, and since the client's own path is discarded and its headers
// are not forwarded, a caller has no channel to inject or override it.
// Deploy this listener only behind a trusted gateway that sets this parameter.
const requestedTenantParam = "tenant_id"

// Scitrera-specific extra header names emitted via ResolvedIdentity.ExtraHeaders
// on the authenticated path. Declared once here so Resolve() (emit) and
// ExtraHeaderNames() (clear-on-anonymous) share a single source of truth and
// cannot drift.
const (
	headerScitreraName          = "X-Scitrera-Name"
	headerScitreraUser          = "X-Scitrera-User"
	headerScitreraDefaultTenant = "X-Scitrera-Default-Tenant"
	headerScitreraTenants       = "X-Scitrera-Tenants"
)

// scitreraExtraHeaderNames is the full set of ExtraHeaders this resolver can
// emit. Used to clear these headers on the anonymous ext_authz passthrough.
var scitreraExtraHeaderNames = []string{
	headerScitreraName,
	headerScitreraUser,
	headerScitreraDefaultTenant,
	headerScitreraTenants,
}

// Name implements pkgauthproxy.IdentityResolver.
func (r *Resolver) Name() string { return "scitrera_mt" }

// ExtraHeaderNames implements pkgauthproxy.ExtraHeaderNaming. The auth-proxy
// clears these (empty override) on the anonymous verify-optional passthrough so
// a client-supplied X-Scitrera-* cannot survive to the backend.
func (r *Resolver) ExtraHeaderNames() []string { return scitreraExtraHeaderNames }

// Resolve implements pkgauthproxy.IdentityResolver.
//
// Machine principals (api_key, task_token) bypass MT lookup entirely — they
// already represent a trusted identity. User-presented credentials run the
// full Python parity flow.
func (r *Resolver) Resolve(ctx context.Context, in pkgauthproxy.ResolverInput) (*pkgauthproxy.ResolvedIdentity, error) {
	// Machine principal pass-through.
	if !isUserMethod(in.Method) {
		// Machine principals have no MT user fields. Explicit empty overrides
		// prevent client-supplied Scitrera user headers reaching the backend.
		cleared := make(map[string]string, len(scitreraExtraHeaderNames))
		for _, name := range scitreraExtraHeaderNames {
			cleared[name] = ""
		}
		return &pkgauthproxy.ResolvedIdentity{
			UserID:        in.Identity.ID,
			PrincipalType: string(in.Identity.Type),
			ExtraHeaders:  cleared,
		}, nil
	}

	if r.revisionGate != nil {
		if err := r.revisionGate.Check(ctx); err != nil {
			return nil, fmt.Errorf("auth revision unavailable: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	email := strings.ToLower(emailFromClaims(in.Claims, in.Identity.ID))
	if email == "" || !strings.Contains(email, "@") {
		return reject(http.StatusUnauthorized, "missing_email", "no email available for MT lookup"), nil
	}
	provider := stringClaim(in.Claims, "provider")

	user, err := r.userCache.Get(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("mt user lookup: %w", err)
	}

	if user != nil && !user.Enabled {
		return reject(http.StatusForbidden, "user_disabled", "user disabled in MT"), nil
	}
	requested := requestedTenant(in.Request)
	if user == nil || requested == "" || !slices.Contains(user.TenantSlugs, requested) {
		user, err = r.autoAdd(ctx, user, email, provider, requested, in.Claims)
		if err != nil {
			return nil, fmt.Errorf("tenant auto-add: %w", err)
		}
	}
	if user == nil {
		return reject(http.StatusUnauthorized, "membership_required", "existing tenant membership or eligible auto-add required"), nil
	}
	defaultTenant := user.DefaultTenantSlug
	tenants := arraysToTenants(user.TenantSlugs, user.TenantNames, user.TenantLogos, user.TenantDefaultWS)

	// Per-tenant auth_checks evaluation.
	tenants, defaultTenant, dropped := r.applyAuthChecks(ctx, tenants, defaultTenant, provider, in.Claims, true)
	if len(tenants) == 0 {
		r.log.Warn().Str("email", email).Strs("dropped", dropped).Msg("scitrera_mt: no tenants survived auth_checks")
		return reject(http.StatusUnauthorized, "auth_checks_failed",
			"no tenants survived auth_checks"), nil
	}

	// Requested-tenant gate: THE tenant-membership check for tenant-scoped
	// gates. Evaluated against the post-auth_checks list, so a tenant the user
	// belongs to but was dropped for this provider is correctly unreachable.
	//
	// This lives here rather than in the ACL because membership is MT data
	// (user_tenants) that only this resolver loads — encoding it as acl_rules
	// rows would mean dual-writing membership into a second store and letting
	// the two drift. The ACL stays a coarse "is this gate open to authenticated
	// users at all" check; WHO may enter WHICH tenant is answered here.
	//
	// An ABSENT tenant_id means a non-tenant-scoped gate (superadmin, mlflow,
	// blobgw): skip the check so those surfaces are unaffected. Logged at debug
	// so a tenant route that forgets the parameter is diagnosable.
	if requested := requestedTenant(in.Request); requested != "" {
		if !hasSlug(tenants, requested) {
			r.log.Warn().
				Str("email", email).
				Str("requested_tenant", requested).
				Strs("permitted_tenants", slugList(tenants)).
				Msg("scitrera_mt: requested tenant not permitted for user")
			return reject(http.StatusForbidden, "tenant_not_permitted",
				"not a member of the requested tenant"), nil
		}
		// Pin the identity to the tenant actually being addressed. Without
		// this, a user belonging to several tenants would have their MT
		// DEFAULT tenant injected as X-Auth-Tenant-ID no matter which tenant's
		// route they came in on — mis-attributing the request downstream.
		defaultTenant = requested
	} else {
		r.log.Debug().Str("email", email).Msg("scitrera_mt: gate is not tenant-scoped (no tenant_id)")
	}

	if defaultTenant == "" {
		defaultTenant = tenants[0].Slug
	}

	displayName := stringClaim(in.Claims, "name")
	if user != nil && user.Name != "" {
		displayName = user.Name
	}

	resolved := &pkgauthproxy.ResolvedIdentity{
		UserID:          email,
		DisplayName:     displayName,
		PrincipalType:   "User",
		DefaultTenantID: defaultTenant,
		TenantIDs:       slugList(tenants),
		Tenants:         toPkgTenants(tenants),
		ExtraHeaders: map[string]string{
			headerScitreraName:          displayName,
			headerScitreraUser:          email,
			headerScitreraDefaultTenant: defaultTenant,
			headerScitreraTenants:       strings.Join(slugList(tenants), ","),
		},
	}

	return resolved, nil
}

// applyAuthChecks iterates candidate tenants and drops those whose
// auth:checks:{provider} configuration is not satisfied by the claim set.
// Mirrors the inner loop in main.py:_auth_common (lines 144-160 / 199-214).
func (r *Resolver) applyAuthChecks(
	ctx context.Context,
	tenants []tenantInfo,
	defaultTenant, provider string,
	claims map[string]any,
	registeredMember bool,
) (kept []tenantInfo, newDefault string, dropped []string) {
	if provider == "" {
		// No provider name — auth_checks key is built from provider, so we
		// can't apply per-provider rules. Pass through unchanged.
		return tenants, defaultTenant, nil
	}
	cfgKey := "auth:checks:" + provider
	for _, t := range tenants {
		// Per-tenant permitted-provider allowlist (auth:providers). A non-empty
		// list gates logins to its members: if the login provider isn't in it,
		// drop this tenant. Empty/unset => allow any provider. Evaluated BEFORE the
		// per-provider auth:checks so a disallowed provider never reaches tid/hd.
		if allowed, reason, lookupOK := r.providerAllowed(ctx, t.Slug, provider); !lookupOK {
			r.log.Warn().Str("tenant", t.Slug).Msg("scitrera_mt: auth_providers lookup error")
			// Fail-closed on a transient cfg lookup error (matches auth_checks).
			dropped = append(dropped, t.Slug)
			continue
		} else if !allowed {
			r.log.Info().Str("tenant", t.Slug).Str("reason", reason).Msg("scitrera_mt: tenant dropped by auth_providers")
			dropped = append(dropped, t.Slug)
			continue
		}

		raw, err := r.cfgCache.Get(ctx, t.Slug+":"+cfgKey)
		if err != nil {
			r.log.Warn().Err(err).Str("tenant", t.Slug).Msg("scitrera_mt: cfg lookup error")
			// Fail-closed for this tenant on a transient cfg lookup error.
			dropped = append(dropped, t.Slug)
			continue
		}
		checks, ok := raw.(map[string]any)
		if !ok || len(checks) == 0 {
			// No rules configured → tenant passes.
			kept = append(kept, t)
			continue
		}
		if ok, reason := evaluateTenantChecks(provider, checks, claims, registeredMember); ok {
			kept = append(kept, t)
		} else {
			r.log.Info().Str("tenant", t.Slug).Str("reason", reason).Msg("scitrera_mt: tenant dropped by auth_checks")
			dropped = append(dropped, t.Slug)
		}
	}
	if defaultTenant != "" {
		stillThere := false
		for _, t := range kept {
			if t.Slug == defaultTenant {
				stillThere = true
				break
			}
		}
		if !stillThere {
			defaultTenant = ""
		}
	}
	return kept, defaultTenant, dropped
}

// providerAllowed reports whether `provider` is permitted for `slug` per the
// tenant's `auth:providers` allowlist (tenant_config, a list of provider names).
// A non-empty list gates logins to its members; an empty/unset/wrong-type value
// means "no allowlist" and permits any provider. `lookupOK` is false only on a
// cfg-cache error, so the caller fails closed (drops the tenant) exactly as it
// does for auth:checks.
func (r *Resolver) providerAllowed(ctx context.Context, slug, provider string) (allowed bool, reason string, lookupOK bool) {
	raw, err := r.cfgCache.Get(ctx, slug+":auth:providers")
	if err != nil {
		return false, "auth_providers lookup error", false
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		// Unset / empty / not a list → no allowlist → allow any provider.
		return true, "", true
	}
	for _, v := range list {
		if s, ok := v.(string); ok && s == provider {
			return true, "", true
		}
	}
	return false, fmt.Sprintf("provider %q not in tenant allowlist", provider), true
}

// evaluateTenantChecks permits an explicitly configured blank Google hd option
// only for existing enabled members. Resolve derives their candidate tenants
// from enabled memberships; domain-only candidates must never use this option.
func evaluateTenantChecks(provider string, checks, claims map[string]any, registeredMember bool) (bool, string) {
	want, restricted := checks["hd"]
	if provider == "google" && restricted {
		got, present := claims["hd"]
		hd, stringClaim := got.(string)
		if !present || (stringClaim && hd == "") {
			allowBlank := want == ""
			if options, ok := want.([]any); ok {
				for _, option := range options {
					if value, ok := option.(string); ok && value == "" {
						allowBlank = true
					}
				}
			}
			if !registeredMember || !allowBlank {
				return false, "missing Google hosted domain requires an allowed registered membership"
			}
			// Other claims remain mandatory, and verified input claims are untouched.
			remaining := make(map[string]any, len(checks)-1)
			for key, value := range checks {
				if key != "hd" {
					remaining[key] = value
				}
			}
			return evaluateChecks(remaining, claims)
		}
	}
	return evaluateChecks(checks, claims)
}

// evaluateChecks mirrors `_tenant_auth_check_helper` from
// scitrera_forward_auth/main.py:71-82. The check map shape is:
//
//	{ "tid": ["uuid1", "uuid2"], "hd": "example.com" }
//
// — list values mean "claim must be in this set", scalar values mean
// "claim must equal this string".
func evaluateChecks(checks map[string]any, claims map[string]any) (bool, string) {
	for k, want := range checks {
		got, present := claims[k]
		if !present {
			return false, fmt.Sprintf("missing claim %q", k)
		}
		switch wantTyped := want.(type) {
		case []any:
			gotStr, _ := got.(string)
			if gotStr == "" {
				return false, fmt.Sprintf("claim %q is not a string", k)
			}
			match := false
			for _, v := range wantTyped {
				if s, ok := v.(string); ok && s == gotStr {
					match = true
					break
				}
			}
			if !match {
				return false, fmt.Sprintf("claim %q value not in allowed set", k)
			}
		case string:
			if gotStr, ok := got.(string); !ok || gotStr != wantTyped {
				return false, fmt.Sprintf("claim %q does not equal expected", k)
			}
		default:
			return false, fmt.Sprintf("unsupported check type for claim %q: %T", k, want)
		}
	}
	return true, ""
}

// tenantInfo is the in-resolver representation of a tenant entry.
type tenantInfo struct {
	Slug      string
	Name      string
	Logo      string
	DefaultWS string
}

func arraysToTenants(slugs, names, logos, dw []string) []tenantInfo {
	n := len(slugs)
	out := make([]tenantInfo, 0, n)
	for i := 0; i < n; i++ {
		t := tenantInfo{Slug: slugs[i]}
		if i < len(names) {
			t.Name = names[i]
		}
		if i < len(logos) {
			t.Logo = logos[i]
		}
		if i < len(dw) {
			t.DefaultWS = dw[i]
		}
		out = append(out, t)
	}
	return out
}

// requestedTenant returns the slug named by the gate, lowercased/trimmed to
// match MT slug storage, or "" when the gate is not tenant-scoped (or no
// request is attached, as in unit tests and non-HTTP callers).
func requestedTenant(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get(requestedTenantParam)))
}

// hasSlug reports whether slug is among the tenants the user may use.
func hasSlug(tenants []tenantInfo, slug string) bool {
	for _, t := range tenants {
		if strings.EqualFold(t.Slug, slug) {
			return true
		}
	}
	return false
}

func slugList(tenants []tenantInfo) []string {
	out := make([]string, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, t.Slug)
	}
	return out
}

func toPkgTenants(tenants []tenantInfo) []pkgauthproxy.TenantInfo {
	out := make([]pkgauthproxy.TenantInfo, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, pkgauthproxy.TenantInfo{
			ID: t.Slug, Name: t.Name, Logo: t.Logo, DefaultWorkspace: t.DefaultWS,
		})
	}
	return out
}

func isUserMethod(method string) bool {
	switch method {
	case "oauth", "azure_entra", "session":
		return true
	}
	return false
}

func stringClaim(claims map[string]any, key string) string {
	if v, ok := claims[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func emailFromClaims(claims map[string]any, fallback string) string {
	for _, k := range []string{"email", "upn", "preferred_username"} {
		if e := stringClaim(claims, k); e != "" {
			return e
		}
	}
	return fallback
}

func emailDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// reject yields a ResolvedIdentity whose Reject is set — the middleware
// translates this into an HTTP error response.
func reject(status int, code, msg string) *pkgauthproxy.ResolvedIdentity {
	return &pkgauthproxy.ResolvedIdentity{
		Reject: &pkgauthproxy.Rejection{Status: status, Code: code, Message: msg},
	}
}
