# Testing

Use Go 1.25.14 and the versions documented in README.md. All integration records
are synthetic. Each Go database test creates a uniquely named `auth_test_*`
database and drops only that owned database afterward. The supplied PostgreSQL
role needs CREATEDB. Never point this at production.

```sh
docker run -d --name auth-test-postgres \
  -e POSTGRES_USER=auth -e POSTGRES_PASSWORD=local-test-only -e POSTGRES_DB=auth \
  -p 127.0.0.1:55439:5432 \
  postgres:17.11-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73
export AUTH_TEST_POSTGRES_DSN='postgres://auth:local-test-only@127.0.0.1:55439/auth?sslmode=disable'
make build check
```

Without that DSN, database tests explicitly skip. The full acceptance run must
set it. Unit tests cover legacy tenant/provider/machine/header behavior,
default-off admission, cache generation races, and revision reconnect recovery.
Integration tests cover:

- Empty and legacy schemas, repeat migrations, incompatible schema failures.
- Real MessagePack read/write, original UUIDs, and unrelated config/table survival.
- Operator credentials, expiry/logout, ordinary-user rejection and CSRF/origin.
- Normalized duplicate email/domain conflicts, stale-edit conflicts, transactional
  policy rollback under an injected database failure, memberships/defaults, audit.
- Warm caches on two resolver instances, disable/change/remove propagation.
- A synthetic local RSA-signed OIDC issuer through login, callback, tenant-scoped
  verification, anonymous spoof clearing, public/internal separation and logout.
- A real reverse-proxy backend with a synthetic machine API token and spoofed
  incoming identity headers, preserving the verified machine principal.
- Multiple Google hosted domains and an explicit blank option: enabled members
  without `hd` pass only when selected; unregistered candidates cannot use it;
  wrong/malformed claims, disabled users and missing memberships fail. Real API
  and MessagePack round trips, no auto-created records, and guest-option revocation
  after warmed caches are covered.
- Cross-site navigation to the public sign-in shell succeeds, while cross-site
  API requests, foreign origins and unexpected hosts remain rejected.

## Browser QA

Start the real built service against a disposable migrated database. Set the
admin address to `127.0.0.1:9082` and origin to `http://127.0.0.1:9082`.

```sh
export AUTH_BROWSER_TOKEN_FILE="$PWD/.local/operators.json"
cd web
npx playwright install chromium
npm run test:browser
```

`AUTH_BROWSER_URL` overrides the service URL. `AUTH_CHROMIUM_PATH` can select an
installed Chromium executable, and `AUTH_BROWSER_OUTPUT` chooses the evidence
directory. The full configuration workflow test creates uniquely named example
tenants/users and exercises first-operator instructions, sign-in, CRUD, domain
conflicts, provider checks/clearing, concurrent-edit feedback, memberships/default
removal, disabled status, page refresh persistence, desktop/narrow viewports and
logout. It records synthetic screenshots and rejects browser JavaScript errors.
It also opens the dashboard from a link on another origin and verifies multiple
hosted domains, the explicit blank option and unknown checks survive save/reload
and switching between structured and JSON editors. For a local HTTPS test proxy
with a self-signed certificate, set `AUTH_BROWSER_INSECURE_TLS=1` to let only the
test browser accept it; certificate validation is otherwise enabled by default.

Traces and video are disabled because request traces could capture the operator
token during sign-in. Browser credentials remain outside the source tree and
are not copied into evidence. The browser test operates on a disposable database
and leaves its synthetic records for inspection; delete the owned test deployment
when finished.

## Build isolation and release checks

Run `docker build .` from this repository alone. It resolves only public pinned
inputs and builds the UI and source bundle inside the image. Run the binary's
`version` command and exercise `/admin/` and the session-protected API against a
local test database. Extract the source tarball to an empty directory and run
`make build` there with `GOWORK=off` to check the actual corresponding-source path.

`python3 scripts/check-release.py` verifies version/toolchain/source boundaries.
Release-gate tests run in CI and can be run locally with:

```sh
uv run --no-project --with scitrera-repo-tools==0.1.30 python -m unittest discover -s scripts -p 'test_*.py'
```

They cover tag/version agreement, the publication toggle, manual/branch runs,
and prerelease classification without creating any remote release. Archive tests
exercise Make's real source-generation/check sequence: prohibited files must stop
fresh and incremental builds before compilation, and a missing archive must fail.
They require Node, GNU tar and Make, but do not install npm packages or compile Go.
Container workflow tests verify the native runner/platform mapping, both digest
inputs to the manifest merge, and tag-only publication after all release checks.
`make check-metadata` also checks the reusable container workflow against
repo-tools' generator output.

`python3 scripts/notices.py` refreshes installed dependency license texts.
`npm audit` and a secret scan complement, but do not replace, the explicit export
review. Keep the local verification record with the release evidence;
prepared GitHub workflows are not evidence of executed remote CI.

## Listener, telemetry and navigation coverage

The Go suites cover explicit listener sharing, separate public/admin/metrics
surfaces, metrics fallback/disable, Prometheus without a collector, and actual
OTLP/gRPC export of traces and metrics over TLS using a test CA. Exporter headers
and resource overrides are checked. Database tests cover tenant membership,
search/status filtering, literal matching, pagination, and preservation of other
memberships. Browser QA covers tenant-to-users links, filter/detail deep links,
reload, browser Back/Forward, and history state alongside the configuration flows.

## Auto-add acceptance

Integration tests verify the default-off/API/MessagePack contract, explicit enable
and disable, real user/membership/default/audit persistence, two-instance cache
propagation, concurrent first sign-ins, existing-profile/membership preservation,
and rollback after injected membership failure. Tests warm caches before user or
tenant disable, domain removal, changed rules and malformed flags. A real blocked
transaction verifies policy is reread after a concurrent writer commits.

Synthetic signed OIDC runs both registered-member and automatic-enrollment flows.
Google unhosted/blank accounts, unverified emails, wrong or unconstrained
organization claims, unknown providers and unrelated requested tenants never
enroll. Browser QA verifies the tenant toggle defaults off and survives save,
JSON/structured editing, reload and explicit disable.
