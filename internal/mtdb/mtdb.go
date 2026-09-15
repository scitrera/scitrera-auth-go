// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package mtdb is a thin direct-SQL repository over the Scitrera
// multi-tenant Postgres schema (users, tenants, user_tenants,
// tenant_domains, tenant_config). It mirrors a subset of
// backend/scitrera_app_server/mt/_mto_pgsql_sync.py — only the read paths
// the auth resolver needs. Operator writes live in internal/admin; transactional
// user/membership enrollment is implemented here.
//
// Schema reference: backend/scitrera_app_server/mt/mt.sql
package mtdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/scitrera/scitrera-auth-go/internal/mtdb/msgpack"
)

// User is the projection of users + LATERAL-joined tenants membership we need
// to resolve identity to a tenant context.
type User struct {
	ID                string
	Email             string
	Name              string
	Enabled           bool
	DefaultTenantSlug string
	TenantSlugs       []string
	TenantNames       []string
	TenantLogos       []string
	TenantDefaultWS   []string
}

// Tenant is the slug-keyed tenant view (subset of columns).
type Tenant struct {
	ID        string
	Slug      string
	Name      string
	Enabled   bool
	Logo      string
	DefaultWS string
}

// Repo wraps a Postgres connection pool with the MT-specific read queries.
type Repo struct {
	db *sql.DB
}

// New opens a connection pool to dsn and returns a Repo. Conservative pool
// sizing matches the auth-proxy's needs (low concurrency, short queries).
func New(dsn string) (*Repo, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Repo{db: db}, nil
}

// Close releases the underlying pool.
func (r *Repo) Close() error { return r.db.Close() }

// GetUserWithTenants ports backend/scitrera_app_server/mt/_mto_pgsql_sync.py:
// get_user_with_tenants — joins users to tenants via user_tenants, returning
// arrays of (slug, name, logo, default_workspace) per associated enabled
// tenant. Returns (nil, nil) when the email is not found.
func (r *Repo) GetUserWithTenants(ctx context.Context, email string) (*User, error) {
	return getUserWithTenants(ctx, r.db, email)
}

func getUserWithTenants(ctx context.Context, db queryer, email string) (*User, error) {
	const q = `
SELECT u.id::text                                   AS user_id,
       u.email,
       u.name                                       AS user_name,
       u.enabled                                    AS user_enabled,
       u.default_tenant_slug,
       COALESCE(slugs.tenant_slugs, ARRAY []::text[]) AS tenant_slugs,
       COALESCE(slugs.tenant_names, ARRAY []::text[]) AS tenant_names,
       COALESCE(slugs.tenant_logos, ARRAY []::text[]) AS tenant_logos,
       COALESCE(slugs.tenant_dw,    ARRAY []::text[]) AS tenant_default_workspace
FROM public.users u
LEFT JOIN LATERAL (
    SELECT array_agg(COALESCE(t.slug, '') ORDER BY t.slug)                               AS tenant_slugs,
           array_agg(COALESCE(t.name, '') ORDER BY t.slug)                               AS tenant_names,
           array_agg(COALESCE(t.metadata ->> 'logo', '') ORDER BY t.slug)               AS tenant_logos,
           array_agg(COALESCE(t.metadata ->> 'default_workspace', '') ORDER BY t.slug)  AS tenant_dw
    FROM public.user_tenants ut
    JOIN public.tenants t ON t.id = ut.tenant_id
    WHERE ut.user_id = u.id AND t.enabled = true
) slugs ON TRUE
WHERE lower(btrim(u.email)) = $1`

	var u User
	var defaultTenant sql.NullString
	if err := db.QueryRowContext(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.Enabled, &defaultTenant,
		pqArray(&u.TenantSlugs), pqArray(&u.TenantNames),
		pqArray(&u.TenantLogos), pqArray(&u.TenantDefaultWS),
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get_user_with_tenants: %w", err)
	}
	u.DefaultTenantSlug = defaultTenant.String
	return &u, nil
}

// GetTenantByDomain ports get_tenant_by_domain — joins tenants to
// tenant_domains. Returns (nil, nil) when domain is not registered.
func (r *Repo) GetTenantByDomain(ctx context.Context, domain string) (*Tenant, error) {
	return getTenantByDomain(ctx, r.db, domain)
}

func getTenantByDomain(ctx context.Context, db queryer, domain string) (*Tenant, error) {
	const q = `
SELECT t.id::text, t.slug, t.name, t.enabled,
       COALESCE(t.metadata ->> 'logo', '')              AS logo,
       COALESCE(t.metadata ->> 'default_workspace', '') AS default_ws
FROM public.tenants t
JOIN public.tenant_domains td ON t.slug = td.tenant_slug
WHERE lower(rtrim(btrim(td.domain), '.')) = $1`

	var t Tenant
	if err := db.QueryRowContext(ctx, q, domain).Scan(
		&t.ID, &t.Slug, &t.Name, &t.Enabled, &t.Logo, &t.DefaultWS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get_tenant_by_domain: %w", err)
	}
	return &t, nil
}

// GetTenantBySlug returns presentation metadata for a tenant. It returns
// (nil, nil) when the slug is unknown; callers must check Enabled before use.
func (r *Repo) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	const q = `
SELECT id::text, slug, name, enabled,
       COALESCE(metadata ->> 'logo', ''),
       COALESCE(metadata ->> 'default_workspace', '')
FROM public.tenants
WHERE slug = $1`
	var t Tenant
	if err := r.db.QueryRowContext(ctx, q, slug).Scan(
		&t.ID, &t.Slug, &t.Name, &t.Enabled, &t.Logo, &t.DefaultWS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get_tenant_by_slug: %w", err)
	}
	return &t, nil
}

// GetTenantConfigParam reads tenant_config[tenant_slug,key] and msgpack-
// decodes the BYTEA value. Returns (nil, nil) when the key is absent.
//
// The Python side stores any-type values via msgpack; for this consumer we
// surface raw any so callers can type-assert (typically map[string]any for
// "auth:checks:<provider>").
func (r *Repo) GetTenantConfigParam(ctx context.Context, tenantSlug, key string) (any, error) {
	return getTenantConfigParam(ctx, r.db, tenantSlug, key)
}

func getTenantConfigParam(ctx context.Context, db queryer, tenantSlug, key string) (any, error) {
	const q = `SELECT value FROM public.tenant_config WHERE tenant_slug = $1 AND key = $2`
	var raw []byte
	if err := db.QueryRowContext(ctx, q, tenantSlug, key).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get_tenant_config_param(%s,%s): %w", tenantSlug, key, err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	out, err := msgpack.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("get_tenant_config_param(%s,%s): msgpack decode: %w", tenantSlug, key, err)
	}
	return out, nil
}

// SchemaSmokeTest validates every required column, key, and migration.
func (r *Repo) SchemaSmokeTest(ctx context.Context) error { return r.VerifySchema(ctx) }
