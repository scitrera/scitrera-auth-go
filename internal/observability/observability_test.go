// SPDX-License-Identifier: AGPL-3.0-only
package observability

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	metricscollector "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func isolateTelemetry(t *testing.T) {
	t.Helper()
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "OTEL_EXPORTER_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES"} {
		t.Setenv(key, "")
	}
	oldTraces, oldMetrics, oldPropagation := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTraces)
		otel.SetMeterProvider(oldMetrics)
		otel.SetTextMapPropagator(oldPropagation)
	})
}
func TestPrometheusWithoutCollector(t *testing.T) {
	isolateTelemetry(t)
	telemetry, err := Init(context.Background(), true, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer telemetry.Shutdown(context.Background())
	h := HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "admin")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/auth-admin/v1/status", nil))
	w := httptest.NewRecorder()
	telemetry.Metrics.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "http_server_request_duration") || !strings.Contains(w.Body.String(), `auth_surface="admin"`) {
		t.Fatal("HTTP metrics absent", w.Body.String())
	}
	disabled, err := Init(context.Background(), false, "test")
	if err != nil || disabled.Metrics != nil {
		t.Fatal("disabled Prometheus enabled", err)
	}
	_ = disabled.Shutdown(context.Background())
}

type traceReceiver struct {
	tracecollector.UnimplementedTraceServiceServer
	requests chan *tracecollector.ExportTraceServiceRequest
	headers  chan metadata.MD
}

func (s *traceReceiver) Export(ctx context.Context, r *tracecollector.ExportTraceServiceRequest) (*tracecollector.ExportTraceServiceResponse, error) {
	s.requests <- r
	md, _ := metadata.FromIncomingContext(ctx)
	s.headers <- md
	return &tracecollector.ExportTraceServiceResponse{}, nil
}

type metricReceiver struct {
	metricscollector.UnimplementedMetricsServiceServer
	requests chan *metricscollector.ExportMetricsServiceRequest
}

func (s *metricReceiver) Export(_ context.Context, r *metricscollector.ExportMetricsServiceRequest) (*metricscollector.ExportMetricsServiceResponse, error) {
	s.requests <- r
	return &metricscollector.ExportMetricsServiceResponse{}, nil
}
func TestOTLPExportsOverConfiguredTLS(t *testing.T) {
	isolateTelemetry(t)
	traces := &traceReceiver{requests: make(chan *tracecollector.ExportTraceServiceRequest, 8), headers: make(chan metadata.MD, 8)}
	metrics := &metricReceiver{requests: make(chan *metricscollector.ExportMetricsServiceRequest, 8)}
	grpcServer := grpc.NewServer()
	tracecollector.RegisterTraceServiceServer(grpcServer, traces)
	metricscollector.RegisterMetricsServiceServer(grpcServer, metrics)
	collector := httptest.NewUnstartedServer(grpcServer)
	collector.EnableHTTP2 = true
	collector.StartTLS()
	defer collector.Close()
	defer grpcServer.Stop()
	cert := filepath.Join(t.TempDir(), "collector.crt")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: collector.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", cert)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-test=verified")
	t.Setenv("OTEL_SERVICE_NAME", "auth-test-service")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=test")
	telemetry, err := Init(context.Background(), true, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "https://auth.example.test/auth/callback/google?code=synthetic-secret", nil)
	req.Header.Set("Authorization", "Bearer synthetic-secret")
	req.Header.Set("Cookie", "session=synthetic-secret")
	HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "external").ServeHTTP(httptest.NewRecorder(), req)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := telemetry.Shutdown(ctx); err != nil {
		t.Fatal("OTLP TLS export failed", err)
	}
	select {
	case got := <-traces.requests:
		body := got.String()
		if !strings.Contains(body, "auth-test-service") || !strings.Contains(body, "test-version") || !strings.Contains(body, "auth-go-external") || !strings.Contains(body, "deployment.environment.name") {
			t.Fatal("trace/resource attributes missing", body)
		}
		if strings.Contains(body, "synthetic-secret") {
			t.Fatal("credentials captured in trace")
		}
	case <-ctx.Done():
		t.Fatal("no exported trace")
	}
	select {
	case md := <-traces.headers:
		if md.Get("x-test")[0] != "verified" {
			t.Fatal("exporter headers missing")
		}
	case <-ctx.Done():
		t.Fatal("no exporter headers")
	}
	select {
	case got := <-metrics.requests:
		if len(got.ResourceMetrics) == 0 {
			t.Fatal("empty exported metrics")
		}
	case <-ctx.Done():
		t.Fatal("no exported metrics")
	}
}
func TestUnsupportedOTLPProtocolFailsClearly(t *testing.T) {
	isolateTelemetry(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	if _, err := Init(context.Background(), false, "test"); err == nil || !strings.Contains(err.Error(), "configure grpc") {
		t.Fatal("unsupported protocol silently accepted", err)
	}
}
