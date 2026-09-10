# Operations

## Cache freshness and outages

Every resolver instance probes the committed database revision on the next user
check after a **1-second** freshness interval. The successful timestamp is taken
before the probe, so slow queries do not extend the freshness window. A new
revision purges user, domain and config caches, including cached absence.
In-flight cache loads from an older generation cannot repopulate the cache or
return their stale loaded value; their callers retry in the new generation.

The probe has a **2-second** timeout. If the database is unavailable when a probe
is due, that user check fails closed, and the failure does not renew freshness.
The next check probes again. A recovered instance compares the current revision
and purges even if it missed many changes; there is no notification connection
or reconnect gap. Machine principals retain the upstream path.

The bound applies to user checks beginning at least one second after commit,
not to an already authenticated request currently in progress. Database queries
inside user resolution also have a 5-second overall deadline. A race with a
concurrent configuration commit can still let an already started request use
its earlier candidate membership. This is not instantaneous revocation of active
application sessions or ongoing downstream requests.

All instances must share the same migrated PostgreSQL primary. Read replicas
with replication lag are outside this bound. Do not disable the revision triggers
or reset revisions. The underlying cache TTL defaults to 5m and capacity 16384;
the revision gate overrides that TTL when auth data changes. Startup checks detect
missing/disabled triggers; monitor them if other schema owners can change DDL.

## Operator credentials and sessions

Each named operator token is read from a private JSON file at startup. Keep files
identical on replicas. Default session TTL is 8h; set
`SCITRERA_AUTH_ADMIN_SESSION_TTL` to a duration between 1m and 24h. Sessions and
logout are shared in PostgreSQL. Browser refresh obtains its CSRF token again
without storing credentials locally.

To revoke an operator, remove/rotate their entry and restart all replicas.
Sessions retain a credential fingerprint and will no longer validate under the
new configuration. A replica that has not restarted still holds its old file
in memory, so rotation is complete only after all replicas restart. Individual
logout deletes its session immediately. Expired sessions are deleted during the
next successful login; schedule additional database cleanup for high churn.

The sign-in shell and static assets contain no tenant data and are publicly
readable on the private listener. Data and configuration remain session protected.
Host, Origin, Secure/HttpOnly/SameSite cookies and CSRF checks are enforced in Go.
Opening the public dashboard from a link on another site is supported. Cross-site
API requests remain rejected even when the caller supplies a valid session.
Keep the listener on loopback/private networking behind TLS for remote use.

## Audit, backup and monitoring

Monitor service health on the internal/public `/healthz` endpoints and operator
`/status` after login. `/healthz` is liveness, not a database readiness probe.
Audit retention is operator-managed; back up before pruning old audit rows.
The Compose database volume survives service restarts and ordinary `down`.

OpenTelemetry collector export remains optional: configure
`OTEL_EXPORTER_OTLP_ENDPOINT` for traces and metrics over OTLP/gRPC. Prometheus
`/metrics` defaults to the admin listener (internal when admin is disabled); set
`SCITRERA_AUTH_METRICS_ADDR` for a dedicated port or `off` to disable scraping.
See [observability](observability.md) for TLS, sharing and resource settings. Do not export real claims/tokens to
test artifacts. Aether debug logging can include API token prefixes; keep normal
production logging at info or above unless following an approved diagnostic
procedure.

SIGTERM drains the listeners. Keep migrations outside replica startup. Review
schema.md before sharing a platform database, and releases.md before deploying
any future incompatible migration or dependency upgrade.

## Automatic enrollment

The tenant flag defaults off. Enrollments acquire the shared auth revision lock
and revalidate current policy before writing. New-user, membership, revision and
audit writes commit together. Concurrent enrollments are idempotent; repeated
verification of members does not write. Storage errors fail closed. The actor is
`system:auto-add`, with no token or claim payload in the audit.

Disabling auto-add retains enrolled members. An eligible user whose membership
is removed can rejoin while auto-add is on; disable enrollment before removal
when that is not intended. Disabling the global user prevents enrollment/access
across all tenants. No per-tenant deny list is provided.
