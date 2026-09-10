// SPDX-License-Identifier: AGPL-3.0-only
package resolver

import (
	"context"
	"errors"
	"net/mail"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

func (r *Resolver) autoAdd(ctx context.Context, user *mtdb.User, email, provider, requested string, claims map[string]any) (*mtdb.User, error) {
	// An unhosted Google account can only use an existing membership, even if
	// an operator associated gmail.com or removed all Google claim checks.
	if provider == "" || (provider == "google" && stringClaim(claims, "hd") == "") {
		return user, nil
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return user, nil
	}
	domain := emailDomain(email)
	tenant, err := r.domCache.Get(ctx, domain)
	if err != nil {
		return nil, err
	}
	if tenant == nil || !tenant.Enabled || (requested != "" && requested != tenant.Slug) || (user != nil && slices.Contains(user.TenantSlugs, tenant.Slug)) {
		return user, nil
	}
	flag, err := r.cfgCache.Get(ctx, tenant.Slug+":auth:auto_add")
	if err != nil {
		return nil, err
	}
	if enabled, _ := flag.(bool); !enabled {
		return user, nil
	}
	providers, err := r.cfgCache.Get(ctx, tenant.Slug+":auth:providers")
	if err != nil {
		return nil, err
	}
	checks, err := r.cfgCache.Get(ctx, tenant.Slug+":auth:checks:"+provider)
	if err != nil {
		return nil, err
	}
	authorize := func(providers, checks any) bool { return autoAddAllowed(provider, providers, checks, claims) }
	if !authorize(providers, checks) {
		return user, nil
	}
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, stringClaim(claims, "name")))
	if name == "" {
		name = email
	}
	if len(name) > 200 {
		name = strings.ToValidUTF8(name[:200], "")
	}
	enrolled, err := r.repo.AutoAddUser(ctx, email, name, domain, tenant.Slug, provider, authorize)
	// Invalidate cached absence/stale memberships even if another request
	// enrolled first or a concurrent policy change denied this attempt.
	r.userCache.Invalidate(email)
	if errors.Is(err, mtdb.ErrAutoAddDenied) {
		return user, nil
	}
	return enrolled, err
}

// Organization claims must be explicitly constrained for enrollment. Existing
// member checks retain their legacy behavior; enrollment fails closed on absent
// or malformed policies and never treats a blank hd option as authority.
func autoAddAllowed(provider string, providers, rawChecks any, claims map[string]any) bool {
	if providers != nil {
		list, ok := providers.([]any)
		if !ok {
			return false
		}
		found := len(list) == 0
		for _, entry := range list {
			name, ok := entry.(string)
			if !ok || name == "" {
				return false
			}
			found = found || name == provider
		}
		if !found {
			return false
		}
	}
	checks, ok := rawChecks.(map[string]any)
	if !ok {
		return false
	}
	key := ""
	switch provider {
	case "google":
		verified, _ := claims["email_verified"].(bool)
		if !verified {
			return false
		}
		key = "hd"
	case "azure", "entra", "microsoft":
		key = "tid"
		tid := stringClaim(claims, key)
		if id, err := uuid.Parse(tid); err != nil || id == uuid.Nil || id.String() == "9188040d-6c67-4c5b-b112-36a304b66dad" {
			return false
		}
	default:
		return false
	}
	value := stringClaim(claims, key)
	if value == "" {
		return false
	}
	if _, configured := checks[key]; !configured {
		return false
	}
	allowed, _ := evaluateTenantChecks(provider, checks, claims, false)
	return allowed
}
