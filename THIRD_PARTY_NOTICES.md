# Third-party notices

Scitrera Auth first-party source is AGPL-3.0-only. Dependencies retain the licenses
below. This inventory includes linked Go modules and installed frontend build/test
dependencies. Public locked versions/checksums are in go.mod/go.sum and the web
lockfile. Refresh with `python3 scripts/notices.py` after installing dependencies.

Aether's Apache license also covers adapted definitions in migrations/001_proxy.sql.
The Scitrera UI adaptations are described in NOTICE; the underlying shadcn/ui
conventions retain the [MIT notice](LICENSES/shadcn-ui-MIT.txt).
The Go runtime retains its [license](LICENSES/third-party/Go-LICENSE) and
[patent notice](LICENSES/third-party/Go-PATENTS). Container base-image packages
retain their own licenses; consult the pinned images' system documentation.

| Dependency | Version | License/notice files |
|---|---|---|
| `github.com/MicahParks/jwkset` | `v0.11.0` | [LICENSE](LICENSES/third-party/github.com_MicahParks_jwkset@v0.11.0/LICENSE) |
| `github.com/MicahParks/keyfunc/v3` | `v3.8.0` | [LICENSE](LICENSES/third-party/github.com_MicahParks_keyfunc_v3@v3.8.0/LICENSE) |
| `github.com/beorn7/perks` | `v1.0.1` | [LICENSE](LICENSES/third-party/github.com_beorn7_perks@v1.0.1/LICENSE) |
| `github.com/bmatcuk/doublestar/v4` | `v4.6.1` | [LICENSE](LICENSES/third-party/github.com_bmatcuk_doublestar_v4@v4.6.1/LICENSE) |
| `github.com/casbin/casbin/v3` | `v3.10.0` | [LICENSE](LICENSES/third-party/github.com_casbin_casbin_v3@v3.10.0/LICENSE) |
| `github.com/casbin/govaluate` | `v1.3.0` | [LICENSE](LICENSES/third-party/github.com_casbin_govaluate@v1.3.0/LICENSE) |
| `github.com/cenkalti/backoff/v5` | `v5.0.3` | [LICENSE](LICENSES/third-party/github.com_cenkalti_backoff_v5@v5.0.3/LICENSE) |
| `github.com/cespare/xxhash/v2` | `v2.3.0` | [LICENSE.txt](LICENSES/third-party/github.com_cespare_xxhash_v2@v2.3.0/LICENSE.txt) |
| `github.com/coreos/go-oidc/v3` | `v3.18.0` | [LICENSE](LICENSES/third-party/github.com_coreos_go-oidc_v3@v3.18.0/LICENSE), [NOTICE](LICENSES/third-party/github.com_coreos_go-oidc_v3@v3.18.0/NOTICE) |
| `github.com/dgraph-io/badger/v4` | `v4.9.1` | [LICENSE](LICENSES/third-party/github.com_dgraph-io_badger_v4@v4.9.1/LICENSE) |
| `github.com/dgraph-io/ristretto/v2` | `v2.2.0` | [LICENSE](LICENSES/third-party/github.com_dgraph-io_ristretto_v2@v2.2.0/LICENSE) |
| `github.com/dgryski/go-rendezvous` | `v0.0.0-20200823014737-9f7001d12a5f` | [LICENSE](LICENSES/third-party/github.com_dgryski_go-rendezvous@v0.0.0-20200823014737-9f7001d12a5f/LICENSE) |
| `github.com/dustin/go-humanize` | `v1.0.1` | [LICENSE](LICENSES/third-party/github.com_dustin_go-humanize@v1.0.1/LICENSE) |
| `github.com/felixge/httpsnoop` | `v1.0.4` | [LICENSE.txt](LICENSES/third-party/github.com_felixge_httpsnoop@v1.0.4/LICENSE.txt) |
| `github.com/go-jose/go-jose/v4` | `v4.1.4` | [LICENSE](LICENSES/third-party/github.com_go-jose_go-jose_v4@v4.1.4/LICENSE) |
| `github.com/go-logr/logr` | `v1.4.3` | [LICENSE](LICENSES/third-party/github.com_go-logr_logr@v1.4.3/LICENSE) |
| `github.com/go-logr/stdr` | `v1.2.2` | [LICENSE](LICENSES/third-party/github.com_go-logr_stdr@v1.2.2/LICENSE) |
| `github.com/golang-jwt/jwt/v5` | `v5.3.1` | [LICENSE](LICENSES/third-party/github.com_golang-jwt_jwt_v5@v5.3.1/LICENSE) |
| `github.com/google/flatbuffers` | `v25.2.10+incompatible` | [LICENSE](LICENSES/third-party/github.com_google_flatbuffers@v25.2.10+incompatible/LICENSE) |
| `github.com/google/uuid` | `v1.6.0` | [LICENSE](LICENSES/third-party/github.com_google_uuid@v1.6.0/LICENSE) |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | `v2.29.0` | [LICENSE](LICENSES/third-party/github.com_grpc-ecosystem_grpc-gateway_v2@v2.29.0/LICENSE) |
| `github.com/hashicorp/golang-lru/v2` | `v2.0.7` | [LICENSE](LICENSES/third-party/github.com_hashicorp_golang-lru_v2@v2.0.7/LICENSE) |
| `github.com/klauspost/compress` | `v1.18.5` | [LICENSE](LICENSES/third-party/github.com_klauspost_compress@v1.18.5/LICENSE) |
| `github.com/lib/pq` | `v1.10.9` | [LICENSE.md](LICENSES/third-party/github.com_lib_pq@v1.10.9/LICENSE.md) |
| `github.com/mattn/go-colorable` | `v0.1.13` | [LICENSE](LICENSES/third-party/github.com_mattn_go-colorable@v0.1.13/LICENSE) |
| `github.com/mattn/go-isatty` | `v0.0.20` | [LICENSE](LICENSES/third-party/github.com_mattn_go-isatty@v0.0.20/LICENSE) |
| `github.com/munnerz/goautoneg` | `v0.0.0-20191010083416-a7dc8b61c822` | [LICENSE](LICENSES/third-party/github.com_munnerz_goautoneg@v0.0.0-20191010083416-a7dc8b61c822/LICENSE) |
| `github.com/nats-io/nats.go` | `v1.52.0` | [LICENSE](LICENSES/third-party/github.com_nats-io_nats.go@v1.52.0/LICENSE) |
| `github.com/nats-io/nkeys` | `v0.4.15` | [LICENSE](LICENSES/third-party/github.com_nats-io_nkeys@v0.4.15/LICENSE) |
| `github.com/nats-io/nuid` | `v1.0.1` | [LICENSE](LICENSES/third-party/github.com_nats-io_nuid@v1.0.1/LICENSE) |
| `github.com/prometheus/client_golang` | `v1.23.2` | [LICENSE](LICENSES/third-party/github.com_prometheus_client_golang@v1.23.2/LICENSE), [NOTICE](LICENSES/third-party/github.com_prometheus_client_golang@v1.23.2/NOTICE) |
| `github.com/prometheus/client_model` | `v0.6.2` | [LICENSE](LICENSES/third-party/github.com_prometheus_client_model@v0.6.2/LICENSE), [NOTICE](LICENSES/third-party/github.com_prometheus_client_model@v0.6.2/NOTICE) |
| `github.com/prometheus/common` | `v0.67.5` | [LICENSE](LICENSES/third-party/github.com_prometheus_common@v0.67.5/LICENSE), [NOTICE](LICENSES/third-party/github.com_prometheus_common@v0.67.5/NOTICE) |
| `github.com/prometheus/otlptranslator` | `v1.0.0` | [LICENSE](LICENSES/third-party/github.com_prometheus_otlptranslator@v1.0.0/LICENSE) |
| `github.com/prometheus/procfs` | `v0.20.1` | [LICENSE](LICENSES/third-party/github.com_prometheus_procfs@v0.20.1/LICENSE), [NOTICE](LICENSES/third-party/github.com_prometheus_procfs@v0.20.1/NOTICE) |
| `github.com/redis/go-redis/v9` | `v9.17.2` | [LICENSE](LICENSES/third-party/github.com_redis_go-redis_v9@v9.17.2/LICENSE) |
| `github.com/rs/zerolog` | `v1.34.0` | [LICENSE](LICENSES/third-party/github.com_rs_zerolog@v1.34.0/LICENSE) |
| `github.com/scitrera/aether/server` | `v0.2.4` | [LICENSE](LICENSES/third-party/github.com_scitrera_aether_server@v0.2.4/LICENSE) |
| `github.com/vmihailenco/msgpack/v5` | `v5.4.1` | [LICENSE](LICENSES/third-party/github.com_vmihailenco_msgpack_v5@v5.4.1/LICENSE) |
| `github.com/vmihailenco/tagparser/v2` | `v2.0.0` | [LICENSE](LICENSES/third-party/github.com_vmihailenco_tagparser_v2@v2.0.0/LICENSE) |
| `go.opentelemetry.io/auto/sdk` | `v1.2.1` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_auto_sdk@v1.2.1/LICENSE) |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | `v0.69.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_contrib_instrumentation_net_http_otelhttp@v0.69.0/LICENSE) |
| `go.opentelemetry.io/contrib/instrumentation/runtime` | `v0.69.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_contrib_instrumentation_runtime@v0.69.0/LICENSE) |
| `go.opentelemetry.io/otel` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | `v0.19.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_exporters_otlp_otlplog_otlploghttp@v0.19.0/LICENSE) |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_exporters_otlp_otlpmetric_otlpmetricgrpc@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_exporters_otlp_otlptrace@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_exporters_otlp_otlptrace_otlptracegrpc@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/exporters/prometheus` | `v0.66.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_exporters_prometheus@v0.66.0/LICENSE) |
| `go.opentelemetry.io/otel/log` | `v0.19.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_log@v0.19.0/LICENSE) |
| `go.opentelemetry.io/otel/metric` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_metric@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/sdk` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_sdk@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/sdk/log` | `v0.19.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_sdk_log@v0.19.0/LICENSE) |
| `go.opentelemetry.io/otel/sdk/metric` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_sdk_metric@v1.44.0/LICENSE) |
| `go.opentelemetry.io/otel/trace` | `v1.44.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_otel_trace@v1.44.0/LICENSE) |
| `go.opentelemetry.io/proto/otlp` | `v1.10.0` | [LICENSE](LICENSES/third-party/go.opentelemetry.io_proto_otlp@v1.10.0/LICENSE) |
| `go.yaml.in/yaml/v2` | `v2.4.4` | [LICENSE](LICENSES/third-party/go.yaml.in_yaml_v2@v2.4.4/LICENSE), [LICENSE.libyaml](LICENSES/third-party/go.yaml.in_yaml_v2@v2.4.4/LICENSE.libyaml), [NOTICE](LICENSES/third-party/go.yaml.in_yaml_v2@v2.4.4/NOTICE) |
| `golang.org/x/crypto` | `v0.51.0` | [LICENSE](LICENSES/third-party/golang.org_x_crypto@v0.51.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_crypto@v0.51.0/PATENTS) |
| `golang.org/x/net` | `v0.55.0` | [LICENSE](LICENSES/third-party/golang.org_x_net@v0.55.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_net@v0.55.0/PATENTS) |
| `golang.org/x/oauth2` | `v0.36.0` | [LICENSE](LICENSES/third-party/golang.org_x_oauth2@v0.36.0/LICENSE) |
| `golang.org/x/sync` | `v0.21.0` | [LICENSE](LICENSES/third-party/golang.org_x_sync@v0.21.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_sync@v0.21.0/PATENTS) |
| `golang.org/x/sys` | `v0.45.0` | [LICENSE](LICENSES/third-party/golang.org_x_sys@v0.45.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_sys@v0.45.0/PATENTS) |
| `golang.org/x/text` | `v0.39.0` | [LICENSE](LICENSES/third-party/golang.org_x_text@v0.39.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_text@v0.39.0/PATENTS) |
| `golang.org/x/time` | `v0.15.0` | [LICENSE](LICENSES/third-party/golang.org_x_time@v0.15.0/LICENSE), [PATENTS](LICENSES/third-party/golang.org_x_time@v0.15.0/PATENTS) |
| `google.golang.org/genproto/googleapis/api` | `v0.0.0-20260526163538-3dc84a4a5aaa` | [LICENSE](LICENSES/third-party/google.golang.org_genproto_googleapis_api@v0.0.0-20260526163538-3dc84a4a5aaa/LICENSE) |
| `google.golang.org/genproto/googleapis/rpc` | `v0.0.0-20260526163538-3dc84a4a5aaa` | [LICENSE](LICENSES/third-party/google.golang.org_genproto_googleapis_rpc@v0.0.0-20260526163538-3dc84a4a5aaa/LICENSE) |
| `google.golang.org/grpc` | `v1.82.1` | [LICENSE](LICENSES/third-party/google.golang.org_grpc@v1.82.1/LICENSE), [NOTICE.txt](LICENSES/third-party/google.golang.org_grpc@v1.82.1/NOTICE.txt) |
| `google.golang.org/protobuf` | `v1.36.11` | [LICENSE](LICENSES/third-party/google.golang.org_protobuf@v1.36.11/LICENSE), [PATENTS](LICENSES/third-party/google.golang.org_protobuf@v1.36.11/PATENTS) |
| `gopkg.in/yaml.v3` | `v3.0.1` | [LICENSE](LICENSES/third-party/gopkg.in_yaml.v3@v3.0.1/LICENSE), [NOTICE](LICENSES/third-party/gopkg.in_yaml.v3@v3.0.1/NOTICE) |
| `npm:@babel/code-frame` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_code-frame@7.29.7/LICENSE) |
| `npm:@babel/compat-data` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_compat-data@7.29.7/LICENSE) |
| `npm:@babel/core` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_core@7.29.7/LICENSE) |
| `npm:@babel/generator` | `7.29.8` | [LICENSE](LICENSES/third-party/npm_@babel_generator@7.29.8/LICENSE) |
| `npm:@babel/helper-compilation-targets` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-compilation-targets@7.29.7/LICENSE) |
| `npm:@babel/helper-globals` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-globals@7.29.7/LICENSE) |
| `npm:@babel/helper-module-imports` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-module-imports@7.29.7/LICENSE) |
| `npm:@babel/helper-module-transforms` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-module-transforms@7.29.7/LICENSE) |
| `npm:@babel/helper-plugin-utils` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-plugin-utils@7.29.7/LICENSE) |
| `npm:@babel/helper-string-parser` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-string-parser@7.29.7/LICENSE) |
| `npm:@babel/helper-validator-identifier` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-validator-identifier@7.29.7/LICENSE) |
| `npm:@babel/helper-validator-option` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helper-validator-option@7.29.7/LICENSE) |
| `npm:@babel/helpers` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_helpers@7.29.7/LICENSE) |
| `npm:@babel/parser` | `7.29.8` | [LICENSE](LICENSES/third-party/npm_@babel_parser@7.29.8/LICENSE) |
| `npm:@babel/plugin-transform-react-jsx-self` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_plugin-transform-react-jsx-self@7.29.7/LICENSE) |
| `npm:@babel/plugin-transform-react-jsx-source` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_plugin-transform-react-jsx-source@7.29.7/LICENSE) |
| `npm:@babel/template` | `7.29.7` | [LICENSE](LICENSES/third-party/npm_@babel_template@7.29.7/LICENSE) |
| `npm:@babel/traverse` | `7.29.8` | [LICENSE](LICENSES/third-party/npm_@babel_traverse@7.29.8/LICENSE) |
| `npm:@babel/types` | `7.29.8` | [LICENSE](LICENSES/third-party/npm_@babel_types@7.29.8/LICENSE) |
| `npm:@esbuild/linux-arm64` | `0.28.1` | [LICENSE.md](LICENSES/third-party/npm_@esbuild_linux-arm64@0.28.1/LICENSE.md) |
| `npm:@fontsource-variable/geist` | `5.2.8` | [LICENSE](LICENSES/third-party/npm_@fontsource-variable_geist@5.2.8/LICENSE) |
| `npm:@fontsource-variable/jetbrains-mono` | `5.2.8` | [LICENSE](LICENSES/third-party/npm_@fontsource-variable_jetbrains-mono@5.2.8/LICENSE) |
| `npm:@jridgewell/gen-mapping` | `0.3.13` | [LICENSE](LICENSES/third-party/npm_@jridgewell_gen-mapping@0.3.13/LICENSE) |
| `npm:@jridgewell/remapping` | `2.3.5` | [LICENSE](LICENSES/third-party/npm_@jridgewell_remapping@2.3.5/LICENSE) |
| `npm:@jridgewell/resolve-uri` | `3.1.2` | [LICENSE](LICENSES/third-party/npm_@jridgewell_resolve-uri@3.1.2/LICENSE) |
| `npm:@jridgewell/sourcemap-codec` | `1.6.0` | [LICENSE](LICENSES/third-party/npm_@jridgewell_sourcemap-codec@1.6.0/LICENSE) |
| `npm:@jridgewell/trace-mapping` | `0.3.31` | [LICENSE](LICENSES/third-party/npm_@jridgewell_trace-mapping@0.3.31/LICENSE) |
| `npm:@playwright/test` | `1.58.2` | [LICENSE](LICENSES/third-party/npm_@playwright_test@1.58.2/LICENSE), [NOTICE](LICENSES/third-party/npm_@playwright_test@1.58.2/NOTICE) |
| `npm:@radix-ui/react-compose-refs` | `1.1.2` | [MIT](LICENSES/radix-ui-MIT.txt) |
| `npm:@radix-ui/react-slot` | `1.2.3` | [LICENSE](LICENSES/third-party/npm_@radix-ui_react-slot@1.2.3/LICENSE) |
| `npm:@rolldown/pluginutils` | `1.0.0-beta.53` | [LICENSE](LICENSES/third-party/npm_@rolldown_pluginutils@1.0.0-beta.53/LICENSE) |
| `npm:@rollup/rollup-linux-arm64-gnu` | `4.63.1` | [LICENSE.md](LICENSES/third-party/npm_@rollup_rollup-linux-arm64-gnu@4.63.1/LICENSE.md) |
| `npm:@rollup/rollup-linux-arm64-musl` | `4.63.1` | [LICENSE.md](LICENSES/third-party/npm_@rollup_rollup-linux-arm64-musl@4.63.1/LICENSE.md) |
| `npm:@tailwindcss/node` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_@tailwindcss_node@4.1.17/LICENSE) |
| `npm:@tailwindcss/oxide` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_@tailwindcss_oxide@4.1.17/LICENSE) |
| `npm:@tailwindcss/oxide-linux-arm64-gnu` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_@tailwindcss_oxide-linux-arm64-gnu@4.1.17/LICENSE) |
| `npm:@tailwindcss/oxide-linux-arm64-musl` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_@tailwindcss_oxide-linux-arm64-musl@4.1.17/LICENSE) |
| `npm:@tailwindcss/vite` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_@tailwindcss_vite@4.1.17/LICENSE) |
| `npm:@types/babel__core` | `7.20.5` | [LICENSE](LICENSES/third-party/npm_@types_babel__core@7.20.5/LICENSE) |
| `npm:@types/babel__generator` | `7.27.0` | [LICENSE](LICENSES/third-party/npm_@types_babel__generator@7.27.0/LICENSE) |
| `npm:@types/babel__template` | `7.4.4` | [LICENSE](LICENSES/third-party/npm_@types_babel__template@7.4.4/LICENSE) |
| `npm:@types/babel__traverse` | `7.28.0` | [LICENSE](LICENSES/third-party/npm_@types_babel__traverse@7.28.0/LICENSE) |
| `npm:@types/estree` | `1.0.9` | [LICENSE](LICENSES/third-party/npm_@types_estree@1.0.9/LICENSE) |
| `npm:@types/node` | `22.18.10` | [LICENSE](LICENSES/third-party/npm_@types_node@22.18.10/LICENSE) |
| `npm:@types/react` | `19.2.7` | [LICENSE](LICENSES/third-party/npm_@types_react@19.2.7/LICENSE) |
| `npm:@types/react-dom` | `19.2.3` | [LICENSE](LICENSES/third-party/npm_@types_react-dom@19.2.3/LICENSE) |
| `npm:@vitejs/plugin-react` | `5.1.2` | [LICENSE](LICENSES/third-party/npm_@vitejs_plugin-react@5.1.2/LICENSE) |
| `npm:baseline-browser-mapping` | `2.11.21` | [LICENSE.txt](LICENSES/third-party/npm_baseline-browser-mapping@2.11.21/LICENSE.txt) |
| `npm:browserslist` | `4.28.9` | [LICENSE](LICENSES/third-party/npm_browserslist@4.28.9/LICENSE) |
| `npm:caniuse-lite` | `1.0.30001810` | [LICENSE](LICENSES/third-party/npm_caniuse-lite@1.0.30001810/LICENSE) |
| `npm:class-variance-authority` | `0.7.1` | [LICENSE](LICENSES/third-party/npm_class-variance-authority@0.7.1/LICENSE) |
| `npm:clsx` | `2.1.1` | [license](LICENSES/third-party/npm_clsx@2.1.1/license) |
| `npm:convert-source-map` | `2.0.0` | [LICENSE](LICENSES/third-party/npm_convert-source-map@2.0.0/LICENSE) |
| `npm:csstype` | `3.2.3` | [LICENSE](LICENSES/third-party/npm_csstype@3.2.3/LICENSE) |
| `npm:debug` | `4.4.3` | [LICENSE](LICENSES/third-party/npm_debug@4.4.3/LICENSE) |
| `npm:detect-libc` | `2.1.2` | [LICENSE](LICENSES/third-party/npm_detect-libc@2.1.2/LICENSE) |
| `npm:electron-to-chromium` | `1.5.425` | [LICENSE](LICENSES/third-party/npm_electron-to-chromium@1.5.425/LICENSE) |
| `npm:enhanced-resolve` | `5.24.5` | [LICENSE](LICENSES/third-party/npm_enhanced-resolve@5.24.5/LICENSE) |
| `npm:esbuild` | `0.28.1` | [LICENSE.md](LICENSES/third-party/npm_esbuild@0.28.1/LICENSE.md) |
| `npm:escalade` | `3.2.0` | [license](LICENSES/third-party/npm_escalade@3.2.0/license) |
| `npm:fdir` | `6.5.0` | [LICENSE](LICENSES/third-party/npm_fdir@6.5.0/LICENSE) |
| `npm:gensync` | `1.0.0-beta.2` | [LICENSE](LICENSES/third-party/npm_gensync@1.0.0-beta.2/LICENSE) |
| `npm:get-tsconfig` | `4.14.3` | [LICENSE](LICENSES/third-party/npm_get-tsconfig@4.14.3/LICENSE) |
| `npm:graceful-fs` | `4.2.11` | [LICENSE](LICENSES/third-party/npm_graceful-fs@4.2.11/LICENSE) |
| `npm:jiti` | `2.7.0` | [LICENSE](LICENSES/third-party/npm_jiti@2.7.0/LICENSE) |
| `npm:js-tokens` | `4.0.0` | [LICENSE](LICENSES/third-party/npm_js-tokens@4.0.0/LICENSE) |
| `npm:jsesc` | `3.1.0` | [LICENSE-MIT.txt](LICENSES/third-party/npm_jsesc@3.1.0/LICENSE-MIT.txt) |
| `npm:json5` | `2.2.3` | [LICENSE.md](LICENSES/third-party/npm_json5@2.2.3/LICENSE.md) |
| `npm:lightningcss` | `1.30.2` | [LICENSE](LICENSES/third-party/npm_lightningcss@1.30.2/LICENSE) |
| `npm:lightningcss-linux-arm64-gnu` | `1.30.2` | [LICENSE](LICENSES/third-party/npm_lightningcss-linux-arm64-gnu@1.30.2/LICENSE) |
| `npm:lightningcss-linux-arm64-musl` | `1.30.2` | [LICENSE](LICENSES/third-party/npm_lightningcss-linux-arm64-musl@1.30.2/LICENSE) |
| `npm:lru-cache` | `5.1.1` | [LICENSE](LICENSES/third-party/npm_lru-cache@5.1.1/LICENSE) |
| `npm:lucide-react` | `0.559.0` | [LICENSE](LICENSES/third-party/npm_lucide-react@0.559.0/LICENSE) |
| `npm:magic-string` | `0.30.21` | [LICENSE](LICENSES/third-party/npm_magic-string@0.30.21/LICENSE) |
| `npm:ms` | `2.1.3` | [license.md](LICENSES/third-party/npm_ms@2.1.3/license.md) |
| `npm:nanoid` | `3.3.18` | [LICENSE](LICENSES/third-party/npm_nanoid@3.3.18/LICENSE) |
| `npm:node-releases` | `2.0.55` | [LICENSE](LICENSES/third-party/npm_node-releases@2.0.55/LICENSE) |
| `npm:picocolors` | `1.1.1` | [LICENSE](LICENSES/third-party/npm_picocolors@1.1.1/LICENSE) |
| `npm:picomatch` | `4.0.7` | [LICENSE](LICENSES/third-party/npm_picomatch@4.0.7/LICENSE) |
| `npm:playwright` | `1.58.2` | [LICENSE](LICENSES/third-party/npm_playwright@1.58.2/LICENSE), [NOTICE](LICENSES/third-party/npm_playwright@1.58.2/NOTICE) |
| `npm:playwright-core` | `1.58.2` | [LICENSE](LICENSES/third-party/npm_playwright-core@1.58.2/LICENSE), [NOTICE](LICENSES/third-party/npm_playwright-core@1.58.2/NOTICE) |
| `npm:postcss` | `8.5.28` | [LICENSE](LICENSES/third-party/npm_postcss@8.5.28/LICENSE) |
| `npm:prettier` | `3.6.2` | [LICENSE](LICENSES/third-party/npm_prettier@3.6.2/LICENSE) |
| `npm:react` | `19.2.1` | [LICENSE](LICENSES/third-party/npm_react@19.2.1/LICENSE) |
| `npm:react-dom` | `19.2.1` | [LICENSE](LICENSES/third-party/npm_react-dom@19.2.1/LICENSE) |
| `npm:react-refresh` | `0.18.0` | [LICENSE](LICENSES/third-party/npm_react-refresh@0.18.0/LICENSE) |
| `npm:resolve-pkg-maps` | `1.0.0` | [LICENSE](LICENSES/third-party/npm_resolve-pkg-maps@1.0.0/LICENSE) |
| `npm:rollup` | `4.63.1` | [LICENSE.md](LICENSES/third-party/npm_rollup@4.63.1/LICENSE.md) |
| `npm:scheduler` | `0.27.0` | [LICENSE](LICENSES/third-party/npm_scheduler@0.27.0/LICENSE) |
| `npm:semver` | `6.3.1` | [LICENSE](LICENSES/third-party/npm_semver@6.3.1/LICENSE) |
| `npm:source-map-js` | `1.2.1` | [LICENSE](LICENSES/third-party/npm_source-map-js@1.2.1/LICENSE) |
| `npm:tailwind-merge` | `3.5.0` | [LICENSE.md](LICENSES/third-party/npm_tailwind-merge@3.5.0/LICENSE.md) |
| `npm:tailwindcss` | `4.1.17` | [LICENSE](LICENSES/third-party/npm_tailwindcss@4.1.17/LICENSE) |
| `npm:tapable` | `2.3.3` | [LICENSE](LICENSES/third-party/npm_tapable@2.3.3/LICENSE) |
| `npm:tinyglobby` | `0.2.17` | [LICENSE](LICENSES/third-party/npm_tinyglobby@0.2.17/LICENSE) |
| `npm:tsx` | `4.21.0` | [LICENSE](LICENSES/third-party/npm_tsx@4.21.0/LICENSE) |
| `npm:tw-animate-css` | `1.4.0` | [LICENSE](LICENSES/third-party/npm_tw-animate-css@1.4.0/LICENSE) |
| `npm:typescript` | `5.9.3` | [LICENSE.txt](LICENSES/third-party/npm_typescript@5.9.3/LICENSE.txt) |
| `npm:undici-types` | `6.21.0` | [LICENSE](LICENSES/third-party/npm_undici-types@6.21.0/LICENSE) |
| `npm:update-browserslist-db` | `1.3.2` | [LICENSE](LICENSES/third-party/npm_update-browserslist-db@1.3.2/LICENSE) |
| `npm:vite` | `7.3.6` | [LICENSE.md](LICENSES/third-party/npm_vite@7.3.6/LICENSE.md) |
| `npm:yallist` | `3.1.1` | [LICENSE](LICENSES/third-party/npm_yallist@3.1.1/LICENSE) |
