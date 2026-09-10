// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb/msgpack"
)

type Store struct{ DB *sql.DB }
type Envelope struct {
	Revision int64 `json:"revision"`
	Data     any   `json:"data"`
}
type Tenant struct {
	ID       string         `json:"id"`
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Metadata map[string]any `json:"metadata"`
}
type User struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	DefaultTenant string   `json:"default_tenant_slug"`
	Memberships   []string `json:"memberships"`
}
type Policy struct {
	AutoAdd   bool           `json:"auto_add"`
	Providers []string       `json:"providers"`
	Checks    map[string]any `json:"checks"`
}

func (s *Store) read(ctx context.Context, fn func(*sql.Tx) (any, error)) (*Envelope, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton`).Scan(&revision); err != nil {
		return nil, err
	}
	data, err := fn(tx)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &Envelope{revision, data}, nil
}

// A global revision intentionally conflicts even across different resources.
// This small registry favors conservative conflicts over lost writes. The same
// lock is acquired by BEFORE STATEMENT triggers on external platform writes.
func (s *Store) write(ctx context.Context, expected int64, actor, action, resource string, fields []string, fn func(*sql.Tx) error) (*Envelope, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton FOR UPDATE`).Scan(&revision); err != nil {
		return nil, err
	}
	if revision != expected {
		return nil, &problem{409, "revision_conflict", "Configuration changed. Reload before saving your edits."}
	}
	if err = fn(tx); err != nil {
		return nil, mapDBError(err)
	}
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton`).Scan(&revision); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO public.auth_admin_audit(operator,action,resource,fields,revision) VALUES($1,$2,$3,$4,$5)`, actor, action, resource, pq.Array(fields), revision); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, mapDBError(err)
	}
	return &Envelope{revision, map[string]bool{"saved": true}}, nil
}
func mapDBError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return missing()
	}
	var pe *pq.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return &problem{409, "duplicate", "That normalized email, domain or tenant slug already exists"}
		case "23503":
			return invalid("Referenced tenant or user does not exist")
		case "40001", "40P01":
			return &problem{409, "revision_conflict", "Concurrent change. Reload and try again."}
		}
	}
	return err
}
func exists(ctx context.Context, tx *sql.Tx, table, key, value string) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM public."+table+" WHERE "+key+"=$1)", value).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return missing()
	}
	return nil
}

type listFilters struct{ Query, Enabled, Tenant string }

func tenants(ctx context.Context, tx *sql.Tx, slug string, limit, offset int, filters listFilters) (any, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id::text,slug,name,enabled,jsonb_strip_nulls(jsonb_build_object('logo',metadata->'logo','default_workspace',metadata->'default_workspace')) FROM public.tenants WHERE ($1='' OR slug=$1) AND ($4='' OR strpos(lower(name),lower($4))>0 OR strpos(lower(slug),lower($4))>0) AND ($5='' OR enabled=($5='true')) ORDER BY slug LIMIT $2 OFFSET $3`, slug, limit, offset, filters.Query, filters.Enabled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tenant{}
	for rows.Next() {
		var t Tenant
		var raw []byte
		if err = rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Enabled, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &t.Metadata); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if slug != "" {
		if len(out) == 0 {
			return nil, missing()
		}
		return out[0], nil
	}
	return out, nil
}
func users(ctx context.Context, tx *sql.Tx, id string, limit, offset int, filters listFilters) (any, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.id::text,u.email,u.name,u.enabled,COALESCE(u.default_tenant_slug,''),COALESCE((SELECT array_agg(t.slug ORDER BY t.slug) FROM public.user_tenants ut JOIN public.tenants t ON ut.tenant_id=t.id WHERE ut.user_id=u.id),ARRAY[]::text[]) FROM public.users u WHERE ($1='' OR u.id::text=$1) AND ($4='' OR strpos(lower(u.email),lower($4))>0 OR strpos(lower(u.name),lower($4))>0) AND ($5='' OR u.enabled=($5='true')) AND ($6='' OR EXISTS(SELECT 1 FROM public.user_tenants membership JOIN public.tenants tenant ON tenant.id=membership.tenant_id WHERE membership.user_id=u.id AND tenant.slug=$6)) ORDER BY u.email LIMIT $2 OFFSET $3`, id, limit, offset, filters.Query, filters.Enabled, filters.Tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Email, &u.Name, &u.Enabled, &u.DefaultTenant, pq.Array(&u.Memberships)); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if id != "" {
		if len(out) == 0 {
			return nil, missing()
		}
		return out[0], nil
	}
	return out, nil
}
func domains(ctx context.Context, tx *sql.Tx, slug string) (any, error) {
	if err := exists(ctx, tx, "tenants", "slug", slug); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT domain FROM public.tenant_domains WHERE tenant_slug=$1 ORDER BY domain`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func policy(ctx context.Context, tx *sql.Tx, slug string) (any, error) {
	if err := exists(ctx, tx, "tenants", "slug", slug); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT key,value FROM public.tenant_config WHERE tenant_slug=$1 AND (key IN ('auth:providers','auth:auto_add') OR key LIKE 'auth:checks:%') ORDER BY key`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p := Policy{Providers: []string{}, Checks: map[string]any{}}
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		value, e := msgpack.Decode(raw)
		if e != nil {
			return nil, fmt.Errorf("invalid MessagePack auth config")
		}
		if key == "auth:auto_add" {
			// Only an explicit MessagePack boolean true enables enrollment.
			p.AutoAdd, _ = value.(bool)
		} else if key == "auth:providers" {
			if a, ok := value.([]any); ok {
				for _, item := range a {
					v, ok := item.(string)
					if !ok {
						return nil, fmt.Errorf("invalid provider list")
					}
					p.Providers = append(p.Providers, v)
				}
			} else if value != nil {
				return nil, fmt.Errorf("invalid provider list")
			}
		} else {
			p.Checks[key[len("auth:checks:"):]] = value
		}
	}
	return p, rows.Err()
}
func savePolicy(ctx context.Context, tx *sql.Tx, slug string, p PolicyInput) error {
	if err := exists(ctx, tx, "tenants", "slug", slug); err != nil {
		return err
	}
	put := func(key string, value any) error {
		raw, err := msgpack.Encode(value)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO public.tenant_config(tenant_slug,key,value) VALUES($1,$2,$3) ON CONFLICT(tenant_slug,key) DO UPDATE SET value=EXCLUDED.value`, slug, key, raw)
		return err
	}
	if p.AutoAdd != nil {
		if err := put("auth:auto_add", *p.AutoAdd); err != nil {
			return err
		}
	}
	if p.Providers != nil {
		if err := put("auth:providers", *p.Providers); err != nil {
			return err
		}
	}
	for provider, checks := range p.Checks {
		key := "auth:checks:" + provider
		if len(checks) == 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM public.tenant_config WHERE tenant_slug=$1 AND key=$2`, slug, key); err != nil {
				return err
			}
		} else if err := put(key, checks); err != nil {
			return err
		}
	}
	return nil
}
func saveTenant(ctx context.Context, tx *sql.Tx, slug string, input TenantInput, create bool) error {
	if !create {
		if err := exists(ctx, tx, "tenants", "slug", slug); err != nil {
			return err
		}
	} else {
		slug = input.Slug
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	raw, _ := json.Marshal(metadata)
	remove := []string{}
	for key, value := range metadata {
		if string(value) == "null" {
			remove = append(remove, key)
		}
	}
	var err error
	if create {
		_, err = tx.ExecContext(ctx, `INSERT INTO public.tenants(slug,name,enabled,metadata) VALUES($1,$2,$3,jsonb_strip_nulls($4::jsonb))`, slug, input.Name, *input.Enabled, string(raw))
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE public.tenants SET name=$2,enabled=$3,metadata=(COALESCE(metadata,'{}'::jsonb)||$4::jsonb)-$5::text[] WHERE slug=$1`, slug, input.Name, *input.Enabled, string(raw), pq.Array(remove))
	}
	if err != nil {
		return err
	}
	if !*input.Enabled {
		_, err = tx.ExecContext(ctx, `UPDATE public.users SET default_tenant_slug=NULL WHERE default_tenant_slug=$1`, slug)
	}
	return err
}
func validDefault(ctx context.Context, tx *sql.Tx, id, slug string) error {
	if slug == "" {
		return nil
	}
	var ok bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public.user_tenants ut JOIN public.tenants t ON t.id=ut.tenant_id WHERE ut.user_id=$1 AND t.slug=$2 AND t.enabled)`, id, slug).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return invalid("Default tenant must be an enabled membership")
	}
	return nil
}
func saveUser(ctx context.Context, tx *sql.Tx, id string, input UserInput, create bool) error {
	if create {
		if input.DefaultTenant != nil && *input.DefaultTenant != "" {
			return invalid("Create the user and membership before setting a default tenant")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO public.users(id,email,name,enabled) VALUES($1,$2,$3,$4)`, id, input.Email, input.Name, *input.Enabled)
		return err
	}
	if err := exists(ctx, tx, "users", "id::text", id); err != nil {
		return err
	}
	if input.DefaultTenant != nil {
		if err := validDefault(ctx, tx, id, *input.DefaultTenant); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE public.users SET email=$2,name=$3,enabled=$4,default_tenant_slug=CASE WHEN $5::text IS NULL THEN default_tenant_slug ELSE NULLIF($5,'') END WHERE id=$1`, id, input.Email, input.Name, *input.Enabled, input.DefaultTenant)
	return err
}
func membership(ctx context.Context, tx *sql.Tx, id, slug string, remove bool) error {
	if err := exists(ctx, tx, "users", "id::text", id); err != nil {
		return err
	}
	if err := exists(ctx, tx, "tenants", "slug", slug); err != nil {
		return err
	}
	if remove {
		res, err := tx.ExecContext(ctx, `DELETE FROM public.user_tenants WHERE user_id=$1 AND tenant_id=(SELECT id FROM public.tenants WHERE slug=$2)`, id, slug)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return missing()
		}
		_, err = tx.ExecContext(ctx, `UPDATE public.users SET default_tenant_slug=NULL WHERE id=$1 AND default_tenant_slug=$2`, id, slug)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO public.user_tenants(user_id,tenant_id) SELECT $1,id FROM public.tenants WHERE slug=$2`, id, slug)
	return err
}
