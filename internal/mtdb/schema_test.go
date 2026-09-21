// SPDX-License-Identifier: AGPL-3.0-only
package mtdb_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/scitrera/scitrera-auth-go/internal/mtdb/msgpack"
	"github.com/scitrera/scitrera-auth-go/internal/testdb"
)

func TestAdoptLegacySchema(t *testing.T) {
	repo, _ := testdb.New(t)
	ddl, err := os.ReadFile("testdata/platform_mt.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.DB().Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.DB().Exec(`INSERT INTO public.tenants(id,slug,name,metadata) VALUES('11111111-1111-4111-8111-111111111111','legacy','Legacy','{"private_flag":"preserve","nullable":null}'); INSERT INTO public.users(email,name) VALUES('Legacy@Example.COM','Legacy'); INSERT INTO public.user_tenants(user_id,tenant_id) SELECT u.id,t.id FROM public.users u CROSS JOIN public.tenants t; INSERT INTO public.tenant_kv_store(tenant_slug,key,value) VALUES('legacy','unrelated',decode('0102','hex'))`); err != nil {
		t.Fatal(err)
	}
	// Python msgpack.packb({'tid':['synthetic']}) known interoperable fixture.
	raw := []byte{0x81, 0xa3, 't', 'i', 'd', 0x91, 0xa9, 's', 'y', 'n', 't', 'h', 'e', 't', 'i', 'c'}
	if _, err = repo.DB().Exec(`INSERT INTO public.tenant_config(tenant_slug,key,value) VALUES('legacy','auth:checks:azure',$1),('legacy','platform:untouched',$2)`, raw, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = repo.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = repo.DB().Exec(`DELETE FROM auth_proxy.acl_rules`); err != nil {
		t.Fatal(err)
	}
	if err = repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var rules int
	if err = repo.DB().QueryRow(`SELECT count(*) FROM auth_proxy.acl_rules`).Scan(&rules); err != nil || rules != 0 {
		t.Fatal("repeat migration restored an intentionally removed proxy permission", err)
	}
	if err = repo.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	user, err := repo.GetUserWithTenants(context.Background(), "legacy@example.com")
	if err != nil || user == nil {
		t.Fatal("legacy normalized lookup", err)
	}
	if len(user.MembershipChecks["legacy"]) != 0 || len(user.TenantSlugs) != 1 {
		t.Fatal("legacy membership not preserved with empty overrides", user)
	}
	v, err := repo.GetTenantConfigParam(context.Background(), "legacy", "auth:checks:azure")
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["tid"].([]any)[0] != "synthetic" {
		t.Fatal(v)
	}
	var persisted []byte
	if err = repo.DB().QueryRow(`SELECT value FROM public.tenant_config WHERE key='platform:untouched'`).Scan(&persisted); err != nil || string(persisted) != string([]byte{1, 2}) {
		t.Fatal("unrelated config changed")
	}
	var id string
	if err = repo.DB().QueryRow(`SELECT id FROM public.tenants WHERE slug='legacy'`).Scan(&id); err != nil || id != "11111111-1111-4111-8111-111111111111" {
		t.Fatal("existing ID changed")
	}
	if err = repo.DB().QueryRow(`SELECT value FROM public.tenant_kv_store WHERE key='unrelated'`).Scan(&persisted); err != nil {
		t.Fatal("unrelated table changed")
	}
	packed, _ := msgpack.Encode(map[string]any{"hd": "example.com"})
	if _, err = repo.DB().Exec(`UPDATE public.tenant_config SET value=$1 WHERE key='auth:checks:azure'`, packed); err != nil {
		t.Fatal(err)
	}
	v, err = repo.GetTenantConfigParam(context.Background(), "legacy", "auth:checks:azure")
	if err != nil || v.(map[string]any)["hd"] != "example.com" {
		t.Fatal("MessagePack update", err)
	}
}
func TestRejectPartialAndDamagedSchema(t *testing.T) {
	t.Run("partial", func(t *testing.T) {
		repo, _ := testdb.New(t)
		repo.DB().Exec(`CREATE TABLE public.users(email text)`)
		err := repo.Migrate(context.Background())
		if err == nil || !strings.Contains(err.Error(), "partial MT schema") {
			t.Fatal(err)
		}
	})
	t.Run("column", func(t *testing.T) {
		repo, _ := testdb.New(t)
		if err := repo.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		repo.DB().Exec(`ALTER TABLE public.users RENAME COLUMN enabled TO old_enabled`)
		if err := repo.VerifySchema(context.Background()); err == nil || !strings.Contains(err.Error(), "users.enabled") {
			t.Fatal(err)
		}
	})
	t.Run("trigger", func(t *testing.T) {
		repo, _ := testdb.New(t)
		if err := repo.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		repo.DB().Exec(`ALTER TABLE public.users DISABLE TRIGGER auth_admin_revision`)
		if err := repo.VerifySchema(context.Background()); err == nil {
			t.Fatal("disabled revision trigger accepted")
		}
	})
	t.Run("future version", func(t *testing.T) {
		repo, _ := testdb.New(t)
		if err := repo.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		repo.DB().Exec(`INSERT INTO public.auth_admin_migrations(version) VALUES(3)`)
		if err := repo.Migrate(context.Background()); err == nil {
			t.Fatal("future schema accepted")
		}
	})
}
