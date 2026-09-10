-- SPDX-License-Identifier: AGPL-3.0-only
-- Auth subset of the platform MT schema. PostgreSQL 16+.
-- Existing compatible tables and their timestamp triggers remain intact.
CREATE TABLE IF NOT EXISTS public.tenants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), slug text UNIQUE NOT NULL,
 name text NOT NULL, metadata jsonb, enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS public.users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text UNIQUE NOT NULL,
 name text NOT NULL, metadata jsonb, enabled boolean NOT NULL DEFAULT true,
 default_tenant_slug text REFERENCES public.tenants(slug) ON UPDATE CASCADE ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS public.user_tenants (
 user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
 tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(user_id, tenant_id)
);
CREATE TABLE IF NOT EXISTS public.tenant_domains (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 tenant_slug text NOT NULL REFERENCES public.tenants(slug) ON DELETE CASCADE,
 domain text UNIQUE NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS public.tenant_config (
 tenant_slug text NOT NULL REFERENCES public.tenants(slug) ON DELETE CASCADE,
 key text NOT NULL, value bytea,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(tenant_slug, key)
);

-- Refuse ambiguous legacy data rather than silently rewriting identities.
CREATE UNIQUE INDEX auth_admin_users_email_normalized ON public.users (lower(btrim(email)));
CREATE UNIQUE INDEX auth_admin_domains_normalized ON public.tenant_domains (lower(rtrim(btrim(domain), '.')));
CREATE UNIQUE INDEX auth_admin_slugs_normalized ON public.tenants (lower(btrim(slug)));

CREATE TABLE public.auth_admin_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton), revision bigint NOT NULL DEFAULT 0
);
INSERT INTO public.auth_admin_state(singleton) VALUES (true);
CREATE TABLE public.auth_admin_sessions (
 token_hash bytea PRIMARY KEY, operator text NOT NULL, credential_hash bytea NOT NULL,
 csrf_hash bytea NOT NULL, expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX auth_admin_sessions_expiry ON public.auth_admin_sessions(expires_at);
CREATE TABLE public.auth_admin_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 operator text NOT NULL, action text NOT NULL, resource text NOT NULL,
 fields text[] NOT NULL, revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- The singleton serializes all MT writes before row changes, including legacy
-- platform writes. Its revision commits/rolls back with the configuration.
CREATE FUNCTION public.auth_admin_bump_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE public.auth_admin_state SET revision = revision + 1 WHERE singleton;
 RETURN NULL;
END $$;
CREATE FUNCTION public.auth_admin_touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.updated_at = CURRENT_TIMESTAMP;
 RETURN NEW;
END $$;
DO $$
DECLARE t text;
BEGIN
 FOREACH t IN ARRAY ARRAY['tenants','users','user_tenants','tenant_domains','tenant_config'] LOOP
  EXECUTE format('CREATE TRIGGER auth_admin_revision BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON public.%I FOR EACH STATEMENT EXECUTE FUNCTION public.auth_admin_bump_revision()', t);
  IF t <> 'user_tenants' THEN
   EXECUTE format('CREATE TRIGGER auth_admin_updated_at BEFORE UPDATE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.auth_admin_touch_updated_at()', t);
  END IF;
 END LOOP;
END $$;
