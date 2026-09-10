# Scitrera Auth

A standalone Go authentication proxy and operator dashboard for tenant access.
It combines Aether's Apache-licensed OAuth/proxy library with a PostgreSQL tenant
registry and a small React configuration interface. No Python backend is required.

This is a **release candidate**, prepared for publication review.
The repository is `github.com/scitrera/scitrera-auth-go`; [versions.yaml](versions.yaml)
defines the candidate version and shared CI configuration.

The dashboard manages tenants, presentation metadata, domain associations,
provider allowlists and claim checks, users, memberships, and default tenants.
Every API read/write requires a dedicated operator session. Settings persist in
PostgreSQL, with optimistic concurrency and an operator audit trail. User lists
can be filtered by tenant membership, name/email and enabled status. Dashboard
views, selected records, filters and pagination have shareable URLs and support
browser Back/Forward and reload.

## Quick start with Docker

Requires Docker Compose v2.24+, Python 3 for generating local setup secrets, and
network access to public image/module/npm registries during the first build.

```sh
./deploy/setup.sh
```

Open **http://127.0.0.1:8082/admin/**. Sign in using the named operator and token in
`.local/operators.json`. The setup command creates that file privately, runs the
migration, and starts PostgreSQL and the complete service. Re-running setup keeps
existing credentials and data. `deploy/.env` and `.local/` must remain private.

Create a tenant, then create users and their memberships. For automatic enrollment,
associate the intended email domains, configure organization claim checks, and
explicitly enable auto-add in that tenant's sign-in policy. Configure global OIDC clients separately before testing user
sign-in; see [installation](docs/installation.md). The dashboard is usable before
any tenant or OAuth provider exists.

```sh
docker compose --env-file deploy/.env -f deploy/compose.yaml logs auth
docker compose --env-file deploy/.env -f deploy/compose.yaml down
```

`down` retains the database volume. The setup publishes only loopback ports.
Remote use requires a private network and TLS termination for the admin origin.

## Build from source

Use **Go 1.25.14**, Node 24.13.0, npm, GNU tar, Make, and Python 3.11+ with
[scitrera-repo-tools](https://github.com/scitrera/repo-tools) 0.1.30. Install the
build tooling with uv (or `pip install scitrera-repo-tools==0.1.30`):

```sh
uv tool install scitrera-repo-tools==0.1.30
make build
./dist/scitrera-auth-proxy version
make container
```

All dependencies are public and pinned. There are no workspace/sibling replaces
or named build contexts. The frontend is compiled and embedded in the binary.
Docker setup builds do not require repo-tools on the host or in the runtime image.
The build also embeds an allowlisted corresponding-source archive, available from
the dashboard's source link and the public login listener at `/source.tar.gz`.

For Go-only development, `GOWORK=off go test ./...` works before building the UI.
The checked-in embed preparation files prevent a missing-directory build failure.
A Go-only binary serves a preparation page until `make build` compiles the actual
dashboard. It must not be used as a completed dashboard release.

For live frontend development: run the Go service on `127.0.0.1:9082`, set its
admin origin to `http://127.0.0.1:5173`, then run `cd web && npm ci && npm run dev`.
Visit `http://127.0.0.1:5173/admin/`; Vite proxies the same-origin API to Go.

## Admission and propagation

**Auto-add defaults off**, including existing tenants. Access requires an enabled
user and membership; an email-domain association alone grants no access. Enable
per-tenant auto-add to persist eligible organization users and memberships after
Google Workspace or Entra organization checks pass. Existing users retain their
other memberships and defaults; disabled users remain blocked. Turning auto-add
off stops enrollment but retains members already added.

Google hosted-domain policy supports multiple authoritative domains and an
explicit blank option for registered members whose accounts have no `hd` claim.
Blank never enables automatic enrollment. Read [the admission contract](docs/admission.md)
for eligibility, transactional behavior, and the change from earlier domain-only access.

New user checks reflect committed settings within **1 second per instance**.
Revision probes time out after 2 seconds and fail closed when freshness cannot be
confirmed. Requests already in progress may complete using earlier settings.
See [operations](docs/operations.md) for database outages and multiple replicas.

## Documentation and validation

- [Installation and OAuth setup](docs/installation.md)
- [Operator API](docs/api.md) and [schema ownership](docs/schema.md)
- [Admission policy](docs/admission.md), [operations](docs/operations.md), and [metrics/OTEL](docs/observability.md)
- [Testing](docs/testing.md), [release preparation](docs/releases.md), and [source provenance](docs/provenance.md)

`make versions` synchronizes version declarations and build metadata from
`versions.yaml`; `make ci` regenerates the shared workflows. `make check` checks
for version/workflow drift and runs Go race tests, vet, frontend unit
tests/typecheck, and release boundary checks. Database tests require the explicit disposable DSN documented in
the testing guide; browser testing runs against the real service.

First-party code: **AGPL-3.0-only**. Aether and adapted proxy schema: **Apache-2.0**.
See [LICENSE](LICENSE), [NOTICE](NOTICE), [third-party notices](THIRD_PARTY_NOTICES.md),
[contributing](CONTRIBUTING.md), and [security reporting](SECURITY.md).
