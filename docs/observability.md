# Metrics and OpenTelemetry

The service has independent logical surfaces. Use distinct addresses to isolate
admin and metrics from the external load balancer. Identical address strings
explicitly share one listener and dispatch by route; merely using the same port
with different bind addresses does not opt into sharing.

| Surface | Configuration | Suggested address | Exposure |
| --- | --- | --- | --- |
| Verification/reverse proxy | `AUTH_PROXY_LISTEN_ADDR` | `:8080` | Trusted gateway/internal data path |
| Browser login/logout/callback/checkz | `SCITRERA_AUTH_EXTERNAL_ADDR` | `:8081` | External load balancer |
| Operator dashboard/API | `SCITRERA_AUTH_ADMIN_ADDR` | `:8082` | Private network/TLS |
| Prometheus scrape | `SCITRERA_AUTH_METRICS_ADDR` | `:9090` | Monitoring network |

Admin and public login are disabled when their addresses are unset. By default,
`/metrics` follows the admin address; when admin is disabled, it follows the
internal address. A configured metrics address gets its own listener unless it
exactly matches another address. `SCITRERA_AUTH_METRICS_ADDR=off` disables the
Prometheus endpoint without disabling configured OTLP export. No metrics route
is automatically added to the public login listener.

For example, to keep private surfaces separate:

```text
AUTH_PROXY_LISTEN_ADDR=:8080
SCITRERA_AUTH_EXTERNAL_ADDR=:8081
SCITRERA_AUTH_ADMIN_ADDR=:8082
SCITRERA_AUTH_METRICS_ADDR=:9090
```

Point the external load balancer only to 8081. Keep 8080 behind trusted gateways;
8082 and 9090 need private network access. Configure OAuth clients/public redirect
settings and the exact admin HTTPS origin as described in installation.md.
To share admin with the internal data path explicitly, set both addresses to
`:8080`; its `/admin/` and `/api/auth-admin/v1/` routes still require the configured
admin Host/Origin and operator session. Sharing admin with the public listener is
also possible by assigning the same address, and exposes the sign-in shell there.

Scraping `/metrics` requires no operator browser session. It is a monitoring
endpoint protected by the listener's network/ingress boundary. It emits HTTP
request duration/size and Go runtime metrics from the same OpenTelemetry meter
provider used for OTLP. The `auth_surface` label distinguishes internal, external
and admin traffic. Health checks and scrapes are excluded from HTTP telemetry.
There are no user email, token, claim or request-body labels. All metrics listeners
also support `/healthz` for liveness.

## OTLP/gRPC export

OTEL support exports **traces and metrics**, independently of Prometheus scraping.
It is enabled by `OTEL_EXPORTER_OTLP_ENDPOINT`, or the corresponding signal-specific
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` / `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`.
With no endpoint, no collector connection is created.

```text
OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example.com:4317
OTEL_EXPORTER_OTLP_PROTOCOL=grpc
OTEL_SERVICE_NAME=scitrera-auth-go
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production
```

Standard exporter environment settings control headers, certificates, TLS and
signal-specific endpoints. Use `OTEL_EXPORTER_OTLP_CERTIFICATE` for a private CA,
and `OTEL_EXPORTER_OTLP_HEADERS` for collector authentication. For a deliberately
plaintext local collector use `http://127.0.0.1:4317` or the standard insecure
setting. The service no longer forces plaintext over configured TLS. This build
supports OTLP **gRPC**; an explicitly configured unsupported protocol fails at
startup instead of silently choosing a different transport.

Defaults are service.name `scitrera-auth-go`, service.namespace `scitrera`, and the
built service.version. `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES` override
these defaults. SDK sampler/export-interval settings retain their standard
behavior. W3C trace context is propagated. Shutdown flushes both signals with a
bounded deadline. Application logs currently use the existing logging setup;
this module does not promise OTLP log export.

Verification includes a real local HTTP/2 TLS collector receiving trace and metric
requests with configured CA, exporter headers and resource attributes. Tests also
confirm callback query tokens and authorization/cookie values are absent from
exported spans, and that Prometheus works without an OTLP collector.
