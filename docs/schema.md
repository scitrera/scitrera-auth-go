# Schema ownership

`scitrera-auth-proxy migrate` is explicit, serialized with a PostgreSQL advisory
lock, and transactional. It creates version 1 on an empty database or adopts the
complete legacy five-table contract. Run it once per deployment before starting
replicas; a second run is safe. PostgreSQL 16+ is the supported minimum.

The public MT tables remain `tenants`, `users`, `user_tenants`, `tenant_domains`,
and `tenant_config`. UUIDs, primary/unique keys, foreign keys, timestamps, JSONB
metadata, and MessagePack-in-BYTEA configuration are retained. The fresh schema
uses built-in `gen_random_uuid`; the legacy `uuid-ossp` defaults remain when
adopting an existing database. No unrelated MT tables are dropped or rewritten.

The operator API edits only `logo` and `default_workspace` presentation metadata.
Unknown metadata keys remain intact. Other tenant config is never returned or
modified. Auth keys are `auth:providers` and `auth:checks:<provider>`. HTTP JSON is
encoded to MessagePack by Go, not stored as raw JSON bytes.

Auth-owned additions:

- `auth_admin_migrations`: version ledger.
- `auth_admin_state`: singleton committed configuration revision.
- `auth_admin_sessions`: token/CSRF/credential hashes, operator names and expiry.
- `auth_admin_audit`: actor, action, resource, changed field categories, revision,
  timestamp. It contains no token or claim/config payload.
- Normalized unique indexes for emails, domains and slugs.
- `auth_admin_revision` BEFORE STATEMENT triggers on all five MT tables, including
  TRUNCATE, and auth-owned update timestamp triggers.

The startup check verifies required types/nullability, unique keys, foreign keys,
version, admin table projections, normalized indexes and enabled revision
triggers. Partial/incompatible schemas fail explicitly. Duplicates after
normalization fail migration without rewriting identities; resolve those with
the database owner before retrying. Existing application timestamp triggers are
preserved. Required auth triggers must not be disabled by other writers.

External platform writes also acquire the revision lock before changing records
and advance it in the same transaction. Global revisions are conservative: an
unrelated tenant edit can conflict with a pending operator form. Non-auth config
writes also advance the revision; the admin API still never exposes those values.
The revision is an opaque increasing integer, not a count of operator actions.

The isolated `auth_proxy` schema holds selected Aether ACL/API-token/authority
definitions and a proxy schema marker. Its Apache definitions come from Aether
v0.2.3. Only workspace `auth-app` gets a user permission. It does not replace or
modify public Aether tables when an existing Aether DB is configured separately.
Repeat setup retains existing proxy ACL entries; use the migration command only
with the intended database owner, not as a general ACL management operation.

For least-privilege deployment, migrate with a DDL owner, then grant the runtime
role USAGE on `public` and `auth_proxy`; SELECT/INSERT/UPDATE/DELETE on the five MT
tables and admin tables; SELECT on the migration ledger; and sequence usage for
`auth_admin_audit`. The revision trigger needs UPDATE on `auth_admin_state` for
all legacy writers. Proxy operation needs SELECT on its ACL/authority tables and
SELECT/UPDATE on `api_tokens`. Do not grant runtime DDL privileges. The Compose
example uses one disposable local owner for simplicity.

Back up both schema and data, including operator session/audit tables and the
external credential file. Before shared-MT adoption, test against a disposable
copy with the same constraints and validate all downstream writers. This release
does not modify the platform provisioning workflow or claim ownership of its
live database migrations.
