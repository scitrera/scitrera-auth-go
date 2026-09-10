// SPDX-License-Identifier: AGPL-3.0-only
package mtdb

import (
	"context"
	"errors"
	"slices"

	"github.com/lib/pq"
)

var ErrAutoAddDenied = errors.New("tenant auto-add is not permitted")

// AutoAddUser serializes with all operator/platform MT writes using their
// revision lock. Every admission input is reread after acquiring that lock;
// cached policy cannot enroll against a committed disable or domain change.
// authorize evaluates verified provider claims against the fresh policy.
func (r *Repo) AutoAddUser(ctx context.Context, email, name, domain, slug, provider string, authorize func(providers, checks any) bool) (*User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton FOR UPDATE`).Scan(&revision); err != nil {
		return nil, err
	}
	tenant, err := getTenantByDomain(ctx, tx, domain)
	if err != nil {
		return nil, err
	}
	if tenant == nil || !tenant.Enabled || tenant.Slug != slug {
		return nil, ErrAutoAddDenied
	}
	user, err := getUserWithTenants(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if user != nil && !user.Enabled {
		return nil, ErrAutoAddDenied
	}
	flag, err := getTenantConfigParam(ctx, tx, slug, "auth:auto_add")
	if err != nil {
		return nil, err
	}
	if enabled, _ := flag.(bool); !enabled {
		return nil, ErrAutoAddDenied
	}
	providers, err := getTenantConfigParam(ctx, tx, slug, "auth:providers")
	if err != nil {
		return nil, err
	}
	checks, err := getTenantConfigParam(ctx, tx, slug, "auth:checks:"+provider)
	if err != nil {
		return nil, err
	}
	if !authorize(providers, checks) {
		return nil, ErrAutoAddDenied
	}
	// A concurrent sign-in may already have completed enrollment. No extra
	// statements means no duplicate records, audit entries or revision bumps.
	if user != nil && slices.Contains(user.TenantSlugs, slug) {
		return user, tx.Commit()
	}
	fields := []string{"membership"}
	var id string
	if user == nil {
		err = tx.QueryRowContext(ctx, `INSERT INTO public.users(email,name,enabled,default_tenant_slug) VALUES($1,$2,true,$3) RETURNING id::text`, email, name, slug).Scan(&id)
		fields = append(fields, "user")
	} else {
		// Never re-enable a user or overwrite their name, default or other memberships.
		id = user.ID
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO public.user_tenants(user_id,tenant_id) VALUES($1,$2)`, id, tenant.ID); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton`).Scan(&revision); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO public.auth_admin_audit(operator,action,resource,fields,revision) VALUES('system:auto-add','auto_add',$1,$2,$3)`, "users/"+id+"/memberships/"+slug, pq.Array(fields), revision); err != nil {
		return nil, err
	}
	user, err = getUserWithTenants(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	return user, tx.Commit()
}
