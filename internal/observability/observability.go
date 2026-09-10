// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package observability provides OTLP/gRPC traces and metrics, optional
// Prometheus scraping, and HTTP/Go runtime instrumentation.
package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Telemetry struct {
	Metrics  http.Handler
	shutdown []func(context.Context) error
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for _, close := range t.shutdown {
		errs = append(errs, close(ctx))
	}
	return errors.Join(errs...)
}

// Init enables OTLP only when a generic or signal-specific endpoint is set.
// Exporter options deliberately do not override standard TLS/header settings.
// Prometheus works independently of OTLP and never starts its own hidden port.
func Init(ctx context.Context, prometheusEnabled bool, version string) (_ *Telemetry, err error) {
	t := &Telemetry{}
	defer func() {
		if err != nil {
			_ = t.Shutdown(context.Background())
		}
	}()
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "scitrera-auth-go"), attribute.String("service.namespace", "scitrera"), attribute.String("service.version", version)),
		resource.WithFromEnv(), resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}
	readers := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if prometheusEnabled {
		registry := prom.NewRegistry()
		exporter, e := otelprom.New(otelprom.WithRegisterer(registry))
		if e != nil {
			return nil, e
		}
		readers = append(readers, sdkmetric.WithReader(exporter))
		t.Metrics = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	}
	enabled := func(signal string) bool {
		return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT") != ""
	}
	protocol := func(signal string) error {
		value := os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "_PROTOCOL")
		if value == "" {
			value = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		}
		if value != "" && value != "grpc" {
			return fmt.Errorf("OTLP %s protocol %q is unsupported; configure grpc", strings.ToLower(signal), value)
		}
		return nil
	}
	if enabled("TRACES") {
		if err = protocol("TRACES"); err != nil {
			return nil, err
		}
		exporter, e := otlptracegrpc.New(ctx)
		if e != nil {
			return nil, e
		}
		provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
		t.shutdown = append(t.shutdown, provider.Shutdown)
		otel.SetTracerProvider(provider)
	}
	if enabled("METRICS") {
		if err = protocol("METRICS"); err != nil {
			return nil, err
		}
		exporter, e := otlpmetricgrpc.New(ctx)
		if e != nil {
			return nil, e
		}
		readers = append(readers, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	}
	if prometheusEnabled || enabled("METRICS") {
		provider := sdkmetric.NewMeterProvider(readers...)
		t.shutdown = append(t.shutdown, provider.Shutdown)
		otel.SetMeterProvider(provider)
		if err = runtime.Start(runtime.WithMeterProvider(provider)); err != nil {
			return nil, err
		}
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return t, nil
}

// HTTP instruments all functional surfaces while omitting liveness and scrapes.
// Surface values are fixed service labels, never user IDs, tokens or claims.
func HTTP(next http.Handler, surface string) http.Handler {
	return otelhttp.NewHandler(next, "auth-go-"+surface,
		otelhttp.WithSpanNameFormatter(func(operation string, _ *http.Request) string { return operation }),
		otelhttp.WithFilter(func(r *http.Request) bool { return r.URL.Path != "/healthz" && r.URL.Path != "/metrics" }),
		otelhttp.WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
			return []attribute.KeyValue{attribute.String("auth.surface", surface)}
		}),
	)
}
