// SPDX-License-Identifier: AGPL-3.0-only
package mtdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/scitrera/scitrera-auth-go/migrations"
)

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

var contract = map[string]map[string]string{
	"tenants":        {"id": "uuid", "slug": "text", "name": "text", "enabled": "bool", "metadata": "jsonb", "created_at": "timestamptz", "updated_at": "timestamptz"},
	"users":          {"id": "uuid", "email": "text", "name": "text", "enabled": "bool", "metadata": "jsonb", "default_tenant_slug": "text", "created_at": "timestamptz", "updated_at": "timestamptz"},
	"user_tenants":   {"user_id": "uuid", "tenant_id": "uuid", "created_at": "timestamptz"},
	"tenant_domains": {"id": "uuid", "tenant_slug": "text", "domain": "text", "created_at": "timestamptz", "updated_at": "timestamptz"},
	"tenant_config":  {"tenant_slug": "text", "key": "text", "value": "bytea", "created_at": "timestamptz", "updated_at": "timestamptz"},
}

func checkMT(ctx context.Context, q queryer) error {
	for table, columns := range contract {
		for column, typ := range columns {
			var got, nullable string
			var defaultValue sql.NullString
			err := q.QueryRowContext(ctx, `SELECT udt_name,is_nullable,column_default FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&got, &nullable, &defaultValue)
			if err != nil || got != typ {
				return fmt.Errorf("incompatible auth schema: public.%s.%s must be %s", table, column, typ)
			}
			if (column == "id" || column == "created_at" || column == "updated_at") && !defaultValue.Valid {
				return fmt.Errorf("incompatible auth schema: %s.%s requires a default", table, column)
			}
			optional := column == "metadata" || column == "default_tenant_slug" || column == "value"
			if !optional && nullable != "NO" {
				return fmt.Errorf("incompatible auth schema: %s.%s must be NOT NULL", table, column)
			}
		}
	}
	// Require exact unique keys, not merely a column that happens to exist.
	for table, keys := range map[string][]string{"tenants": {"id", "slug"}, "users": {"id", "email"}, "user_tenants": {"user_id,tenant_id"}, "tenant_domains": {"id", "domain"}, "tenant_config": {"tenant_slug,key"}} {
		for _, key := range keys {
			var ok bool
			err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_index i WHERE i.indrelid=('public.' || $1)::regclass AND i.indisunique AND i.indisvalid AND i.indpred IS NULL AND i.indexprs IS NULL AND (SELECT string_agg(a.attname,',' ORDER BY k.n) FROM unnest(i.indkey) WITH ORDINALITY k(attnum,n) JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum WHERE k.n<=i.indnkeyatts)=$2)`, table, key).Scan(&ok)
			if err != nil || !ok {
				return fmt.Errorf("incompatible auth schema: %s requires unique (%s)", table, key)
			}
		}
	}
	for _, fk := range [][4]string{{"users", "default_tenant_slug", "tenants", "slug"}, {"user_tenants", "user_id", "users", "id"}, {"user_tenants", "tenant_id", "tenants", "id"}, {"tenant_domains", "tenant_slug", "tenants", "slug"}, {"tenant_config", "tenant_slug", "tenants", "slug"}} {
		var ok bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=c.conkey[1] JOIN pg_attribute b ON b.attrelid=c.confrelid AND b.attnum=c.confkey[1] WHERE c.contype='f' AND c.convalidated AND c.conrelid=('public.'||$1)::regclass AND a.attname=$2 AND c.confrelid=('public.'||$3)::regclass AND b.attname=$4 AND cardinality(c.conkey)=1 AND c.confdeltype=CASE WHEN $2='default_tenant_slug' THEN 'n'::char ELSE 'c'::char END)`, fk[0], fk[1], fk[2], fk[3]).Scan(&ok)
		if err != nil || !ok {
			return fmt.Errorf("incompatible auth schema: missing foreign key %s.%s", fk[0], fk[1])
		}
	}
	return nil
}

// Migrate adopts a complete compatible MT schema or initializes an empty one.
// Partial schemas are rejected before DDL. All changes are transactional.
func (r *Repo) Migrate(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731954231)`); err != nil {
		return err
	}
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('tenants','users','user_tenants','tenant_domains','tenant_config')`).Scan(&n); err != nil {
		return err
	}
	if n != 0 && n != 5 {
		return fmt.Errorf("partial MT schema: found %d of 5 auth tables; restore a complete compatible schema", n)
	}
	if n == 5 {
		if err = checkMT(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.auth_admin_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0) FROM public.auth_admin_migrations`).Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("unsupported auth schema version %d (binary supports 1)", version)
	}
	if version == 0 {
		raw, _ := migrations.Files.ReadFile("001_auth.sql")
		if _, err = tx.ExecContext(ctx, string(raw)); err != nil {
			return fmt.Errorf("migration 001 failed (check legacy normalized duplicates/schema): %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO public.auth_admin_migrations(version) VALUES(1)`); err != nil {
			return err
		}
	}
	if err = checkSchema(ctx, tx); err != nil {
		return err
	}
	var proxyLedger sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT to_regclass('auth_proxy.schema_version')::text`).Scan(&proxyLedger); err != nil {
		return err
	}
	if !proxyLedger.Valid {
		proxyDDL, _ := migrations.Files.ReadFile("001_proxy.sql")
		if _, err = tx.ExecContext(ctx, string(proxyDDL)); err != nil {
			return fmt.Errorf("proxy schema setup: %w", err)
		}
	}
	if err = checkProxySchema(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func checkProxySchema(ctx context.Context, q queryer) error {
	var version, count int
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(max(version),0),count(*) FROM auth_proxy.schema_version`).Scan(&version, &count); err != nil || version != 1 || count != 1 {
		return fmt.Errorf("proxy schema version 1 required; run scitrera-auth-proxy migrate")
	}
	for table, cols := range map[string]string{
		"acl_rules":             "rule_id,principal_type,principal_id,resource_type,resource_id,access_level,expires_at",
		"acl_fallback_policies": "rule_category,fallback_access_level",
		"api_tokens":            "id,token_hash,name,principal_type,workspace_patterns,scopes,created_by,expires_at,last_used_at,revoked,revoked_at,created_at,updated_at",
		"acl_groups":            "group_id,group_name", "acl_roles": "role_id,role_name",
		"acl_group_members":    "group_id,member_type,member_id,expires_at",
		"acl_role_assignments": "role_id,assignee_type,assignee_id,expires_at",
		"acl_authority_grants": "grant_id,root_grant_id,subject_type,subject_id,delegate_type,delegate_id,issued_by_type,issued_by_id,root_subject_type,root_subject_id,parent_grant_id,may_delegate,remaining_hops,workspace_scope,resource_scope,operation_scope,max_access_level,audience_type,audience_id,valid_while_audience_active,expires_at,renewable_until,renewed_at,revoked,revoked_at,reason,metadata,created_at",
	} {
		rows, err := q.QueryContext(ctx, "SELECT "+cols+" FROM auth_proxy."+table+" LIMIT 0")
		if err != nil {
			return fmt.Errorf("incompatible proxy table %s: %w", table, err)
		}
		rows.Close()
	}
	return nil
}

func checkSchema(ctx context.Context, q queryer) error {
	if err := checkMT(ctx, q); err != nil {
		return err
	}
	var version, count int
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(max(version),0),count(*) FROM public.auth_admin_migrations`).Scan(&version, &count); err != nil || version != 1 || count != 1 {
		return fmt.Errorf("auth schema version 1 required; run scitrera-auth-proxy migrate")
	}
	for table, cols := range map[string]string{"auth_admin_state": "singleton,revision", "auth_admin_sessions": "token_hash,operator,credential_hash,csrf_hash,expires_at,created_at", "auth_admin_audit": "id,operator,action,resource,fields,revision,created_at"} {
		rows, err := q.QueryContext(ctx, "SELECT "+cols+" FROM public."+table+" LIMIT 0")
		if err != nil {
			return fmt.Errorf("incompatible admin table %s: %w", table, err)
		}
		rows.Close()
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM public.auth_admin_state WHERE singleton`).Scan(&count); err != nil || count != 1 {
		return fmt.Errorf("auth revision singleton missing")
	}
	for table := range contract {
		var ok bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=('public.'||$1)::regclass AND tgname='auth_admin_revision' AND tgenabled='O' AND tgfoid='public.auth_admin_bump_revision'::regproc AND tgtype=62)`, table).Scan(&ok)
		if err != nil || !ok {
			return fmt.Errorf("auth revision trigger missing/disabled on %s; restore migration 001", table)
		}
	}
	for _, index := range []string{"auth_admin_users_email_normalized", "auth_admin_domains_normalized", "auth_admin_slugs_normalized"} {
		var ok bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_index WHERE indexrelid=to_regclass($1) AND indisvalid AND indisunique)`, "public."+index).Scan(&ok)
		if err != nil || !ok {
			return fmt.Errorf("auth normalized uniqueness index missing: %s", index)
		}
	}
	return nil
}

func (r *Repo) Revision(ctx context.Context) (int64, error) {
	var v int64
	err := r.db.QueryRowContext(ctx, `SELECT revision FROM public.auth_admin_state WHERE singleton`).Scan(&v)
	return v, err
}

// DB is shared by the auth-only admin store; it does not change pool ownership.
func (r *Repo) DB() *sql.DB { return r.db }

// VerifySchema never migrates at replica startup.
func (r *Repo) VerifySchema(ctx context.Context) error {
	if err := checkSchema(ctx, r.db); err != nil {
		return err
	}
	return checkProxySchema(ctx, r.db)
}

// Normalize stored reads too, so compatible legacy mixed-case identities work.
func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
