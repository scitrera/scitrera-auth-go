# Installation

## Container images

Version-tag releases publish `ghcr.io/scitrera/scitrera-auth-go` with native
`linux/amd64` and `linux/arm64` images under a shared multi-architecture tag.
Docker selects the image matching the host architecture:

```sh
docker pull ghcr.io/scitrera/scitrera-auth-go:0.1.2
docker run --rm ghcr.io/scitrera/scitrera-auth-go:0.1.2 version
```

Use the same environment settings and private operator credential mount described
below and in `deploy/compose.yaml` when running the service. The bundled Compose
quick start builds its image locally; published images can be used in deployments
that do not need a local source build.

## Manual installation

Use PostgreSQL 16+ with an empty dedicated database, or a complete compatible
platform MT database. The migration role needs DDL access; normal service replicas
do not run migrations. See schema.md for role grants and shared-database adoption.

```sh
export SCITRERA_MT_DB_URL='postgres://auth:YOUR_PASSWORD@127.0.0.1:5432/auth?sslmode=require'
./dist/scitrera-auth-proxy migrate
./dist/scitrera-auth-proxy bootstrap --token-file operators.json --operator operator
export SCITRERA_AUTH_ADMIN_TOKEN_FILE="$PWD/operators.json"
export SCITRERA_AUTH_ADMIN_ADDR=127.0.0.1:8082
export SCITRERA_AUTH_ADMIN_ORIGIN=http://127.0.0.1:8082
export AUTH_PROXY_MODE=verify
export AUTH_PROXY_LISTEN_ADDR=127.0.0.1:8080
./dist/scitrera-auth-proxy
```

Open the configured origin at `/admin/`. Bootstrap creates a private JSON file
with `operators: {name: token}`. It refuses overwrites. Tokens contain 32 random
bytes, encoded as base64url. Use the file's values in the sign-in form; do not put
them in URLs, source files, or browser storage. Provision separate names/tokens
for separate operators; every mutation records the name. Additional entries can
be generated with another bootstrap file and combined securely by the operator.

By default, the service uses the same PostgreSQL database for Aether proxy tables
under the `auth_proxy` schema. `migrate` initializes that isolated subset and a
single user gateway permission for workspace `auth-app`. Other ACL fallbacks are
deny. The dashboard manages tenant admission, not general Aether ACL rules or
machine credentials. For an existing Aether deployment, set `AUTH_PROXY_DB_URL`
explicitly to its already migrated database; its ACLs and credentials are reused.

## HTTP surfaces

| Surface | Setting | Purpose |
|---|---|---|
| Verification/proxy | `AUTH_PROXY_LISTEN_ADDR` | Trusted internal gateway; `/auth/verify`, `/auth/verify-optional` or reverse proxy |
| Browser login | `SCITRERA_AUTH_EXTERNAL_ADDR` | Optional public login/callback/logout/checkz |
| Operators | `SCITRERA_AUTH_ADMIN_ADDR` | Optional private dashboard and same-origin admin API |
| Metrics | `SCITRERA_AUTH_METRICS_ADDR` | Optional dedicated `/metrics` listener; otherwise admin, then internal |

Distinct addresses isolate the surfaces. Identical address strings explicitly share
a listener. See [metrics and OTEL](observability.md) for routing examples and the
metrics fallback/disable options.

Administration is disabled when its address is unset. Once set, missing/empty or
insecurely permissioned operator credentials are a startup error. OAuth or machine
credentials never authorize operator API requests.

For remote operators set `SCITRERA_AUTH_ADMIN_ORIGIN=https://auth-admin.example.com`
and put the listener behind a TLS reverse proxy that preserves the configured
Host and Origin. Do not rewrite Origin or expose the listener as a public tenant
route. HTTPS origins use a host-only `__Host-auth_admin` Secure/HttpOnly/Strict
cookie. HTTP origins are restricted to localhost/loopback and use a separate
insecure development cookie. No forwarded header is trusted to infer the origin.

## Global OAuth clients

Tenant provider policy does not create OAuth clients. Register applications with
your chosen OIDC provider, then supply:

```text
AUTH_PROXY_LOGIN_PROVIDERS=azure,google
AUTH_PROXY_LOGIN_AZURE_ISSUER=https://login.microsoftonline.com/organizations/v2.0
AUTH_PROXY_LOGIN_AZURE_CLIENT_ID=...
AUTH_PROXY_LOGIN_AZURE_CLIENT_SECRET=...
AUTH_PROXY_LOGIN_AZURE_REDIRECT_URL=https://auth.example.com/auth/callback/azure
AUTH_PROXY_LOGIN_GOOGLE_ISSUER=https://accounts.google.com
AUTH_PROXY_LOGIN_GOOGLE_CLIENT_ID=...
AUTH_PROXY_LOGIN_GOOGLE_CLIENT_SECRET=...
AUTH_PROXY_LOGIN_GOOGLE_REDIRECT_URL=https://auth.example.com/auth/callback/google
SCITRERA_AUTH_EXTERNAL_ADDR=:8081
SCITRERA_AUTH_POST_LOGIN_TARGET=https://app.example.com
SCITRERA_AUTH_ALLOWED_REDIRECT_HOSTS=app.example.com
SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS=https://app.example.com
```

The Compose service reads an optional `.local/oauth.env`; start with
`deploy/auth.env.example`. When enabling the public listener, add a private
upstream connection from your TLS proxy to port 8081. The default Compose file
does not publish this port. Keep secret files outside source exports; provision
environment values through your deployment's secret manager. The upstream API
currently consumes secret values from environment variables, not `*_FILE` paths.

Redis/Valkey opaque sessions are the default when browser login is enabled.
Compose includes persistent Valkey on a private network without a host port.
The OAuth example selects `AUTH_PROXY_SESSION_STORE=redis` and
`AUTH_PROXY_SESSION_REDIS_ADDR=valkey:6379`. Outside Compose, point every replica
at the same shared primary, DB and prefix. Supported settings include
`AUTH_PROXY_SESSION_REDIS_PASSWORD`, `AUTH_PROXY_SESSION_REDIS_DB` (default `0`),
and `AUTH_PROXY_SESSION_REDIS_PREFIX` (default `auth-session:`). The fallback
address is `AUTH_PROXY_REDIS_ADDR`. Isolate the store and use credentials for
shared deployments. The current client uses a single plain TCP endpoint; direct
TLS, Sentinel and Redis Cluster configuration are not exposed.

Alternatively use
`AUTH_PROXY_SESSION_STORE=jwt` with a random, at least 32-byte
`AUTH_PROXY_SESSION_JWT_SIGNING_KEY`. Signed JWT sessions are stateless: browser
logout clears the cookie but does not revoke a separately retained JWT before
its expiry. Set `AUTH_PROXY_SESSION_TTL` deliberately. Operator sessions always
use the separate revocable PostgreSQL store. JWT mode has no inventory or
per-session server revocation; the dashboard identifies that limitation. Switching
between JWT and Redis requires new sign-ins. See
[session operations](operations.md#browser-sessions) for upgrades and retention.

The upstream OIDC implementation supports configured standards-based providers.
Names correspond exactly to tenant policy keys. The structured editor handles
multiple `azure.tid` and `google.hd` values; other provider/claim names use the JSON
editor. Google hosted-domain options can include an explicit blank option for
existing members without `hd` (including personal Gmail). It never permits
domain-only admission or auto-add. See admission.md for the full policy and
distinction between no restriction, no allowed options, and the blank option.
Personal Google accounts also require an External OAuth audience.
The status page shows configured names and sample supported string claims;
it does not probe provider health. Global client changes require a restart.

Set `AUTH_PROXY_TOKEN_HMAC_KEY` to a random secret (the Compose setup generates
one). This also signs the public post-login return cookie. Configure secure login
cookies and suitable cookie domains for your gateway/application deployment.

## Trusted gateway configuration

The verification listener must be reachable only by trusted gateways. A tenant
route must set `tenant_id` and the ACL `workspace_id` itself and clear incoming
identity/authority headers. `deploy/nginx.conf` demonstrates a route fixed to
tenant `alpha` and workspace `auth-app`. Never let a client control these gate
parameters or call an unscoped gate to reach a tenant-specific application.

In reverse-proxy mode set `AUTH_PROXY_MODE=proxy` and `AUTH_PROXY_BACKEND_URL`.
Aether performs authentication and header injection with the same resolver.

## Enable automatic user enrollment

Auto-add is off by default for both new and adopted tenants. Associate eligible
email domains, configure the tenant's Google hosted-domain or Entra tenant-ID
rules, then enable **Automatically add eligible users to this tenant** and save.
The first eligible authenticated access verification creates the MT user and
membership; an existing enabled user gains only the missing membership. Public
OAuth callback/session creation alone does not enroll. Blank Google hosted-domain
options always require a manually added member. See [admission](admission.md).

When upgrading from the original domain-only admission behavior, access stops until the
user has a membership or the tenant explicitly enables auto-add. This does not
change stored domain associations or existing memberships. The existing migration
version remains valid; no deployment-wide enrollment switch is needed.
