// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func newTestRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(HTTPMiddleware)
	r.Route("/api", func(r chi.Router) {
		r.Get("/containers/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		r.Get("/boom", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	})
	return r
}

func TestHTTPMiddlewareLabelsByRoutePatternNotPath(t *testing.T) {
	router := newTestRouter()
	before := testutil.ToFloat64(httpRequests.WithLabelValues("/api/containers/{id}", http.MethodGet, "2xx"))

	for _, id := range []string{"a1", "b2", "c3", "d4"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/containers/"+id, nil))
	}

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("/api/containers/{id}", http.MethodGet, "2xx")) - before; got != 4 {
		t.Fatalf("four requests to one route must land in one series; got %v", got)
	}
	body := scrape(t)
	for _, id := range []string{"a1", "b2", "c3", "d4"} {
		if strings.Contains(body, "/api/containers/"+id) {
			t.Fatalf("raw path %q leaked into a route label", id)
		}
	}
}

func TestHTTPMiddlewareFoldsUnmatchedPathsAndStatusClass(t *testing.T) {
	router := newTestRouter()
	beforeUnmatched := testutil.ToFloat64(httpRequests.WithLabelValues(unmatchedRoute, http.MethodGet, "4xx"))
	beforeBoom := testutil.ToFloat64(httpRequests.WithLabelValues("/api/boom", http.MethodGet, "5xx"))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/wp-admin/x", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/.env", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/boom", nil))

	if got := testutil.ToFloat64(httpRequests.WithLabelValues(unmatchedRoute, http.MethodGet, "4xx")) - beforeUnmatched; got != 2 {
		t.Fatalf("unmatched probes must share one series; got %v", got)
	}
	if got := testutil.ToFloat64(httpRequests.WithLabelValues("/api/boom", http.MethodGet, "5xx")) - beforeBoom; got != 1 {
		t.Fatalf("a 502 must count under 5xx; got %v", got)
	}
}

func TestBoundedLabelFoldsUnknownValues(t *testing.T) {
	observe := AgentRequestStarted("env-bounded")
	observe("something-new")
	observe(AgentOutcomeOK)

	if got := testutil.ToFloat64(agentRequests.WithLabelValues("env-bounded", "other")); got != 1 {
		t.Fatalf("an unknown outcome must fold into other, once; got %v", got)
	}
	if got := testutil.ToFloat64(agentRequests.WithLabelValues("env-bounded", AgentOutcomeOK)); got != 0 {
		t.Fatalf("the finish func must record exactly once; got %v", got)
	}
	if got := testutil.ToFloat64(agentInFlight.WithLabelValues("env-bounded")); got != 0 {
		t.Fatalf("in-flight must return to zero; got %v", got)
	}
}

func TestAgentReconnectCountsOnlyRepeatConnections(t *testing.T) {
	AgentConnected("env-reconnect")
	AgentConnected("env-reconnect")
	AgentConnected("env-reconnect")

	if got := testutil.ToFloat64(agentConnects.WithLabelValues("env-reconnect")); got != 3 {
		t.Fatalf("connects = %v, want 3", got)
	}
	if got := testutil.ToFloat64(agentReconnects.WithLabelValues("env-reconnect")); got != 2 {
		t.Fatalf("reconnects = %v, want 2", got)
	}
}

func TestScrapeIsValidExpositionAndCarriesEveryFamily(t *testing.T) {
	RegisterAgentPool(func() AgentPoolStats { return AgentPoolStats{Connected: 3, Measuring: 1, Stale: 1} })
	RegisterPlugins(func() (int, int) { return 5, 2 })
	PermissionDenied("metrics.scrape")
	WorkflowRun("manual", "completed", time.Second)
	ScheduleFired("workflow", time.Now().Add(-2*time.Second))
	BackupJobFinished("backup", "success", time.Minute)
	CollectorSweep(time.Second)
	AuditChainVerified(true, 10)
	PluginLoadFailed("addon-kvm")
	AgentOfflineBatch("env-offline", "logs", true, 12, 40, time.Now().Add(-time.Hour))

	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(scrape(t)))
	if err != nil {
		t.Fatalf("scrape is not valid exposition: %v", err)
	}
	for _, name := range []string{
		"mcharbor_http_requests_in_flight",
		"mcharbor_permission_denials_total",
		"mcharbor_agents",
		"mcharbor_agent_offline_buffer_remaining",
		"mcharbor_workflow_runs_total",
		"mcharbor_schedule_lateness_seconds",
		"mcharbor_backup_job_duration_seconds",
		"mcharbor_metrics_collector_sweep_duration_seconds",
		"mcharbor_audit_chain_verify_ok",
		"mcharbor_plugins",
		"mcharbor_plugin_load_failures_total",
		"mcharbor_build_info",
		"go_goroutines",
	} {
		if _, ok := families[name]; !ok {
			t.Errorf("family %s missing from the scrape", name)
		}
	}

	agents := map[string]float64{}
	for _, m := range families["mcharbor_agents"].GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "state" {
				agents[l.GetValue()] = m.GetGauge().GetValue()
			}
		}
	}
	if agents["live"] != 1 || agents["measuring"] != 1 || agents["stale"] != 1 {
		t.Fatalf("agent states = %v, want one of each", agents)
	}
}

func TestGrafanaDashboardOnlyQueriesExportedFamilies(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packaging", "grafana", "mcharbor-control-plane.json"))
	if err != nil {
		t.Fatalf("shipped dashboard unreadable: %v", err)
	}
	var dash struct {
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(raw, &dash); err != nil {
		t.Fatalf("dashboard is not valid JSON: %v", err)
	}

	RegisterAgentPool(func() AgentPoolStats { return AgentPoolStats{} })
	RegisterPlugins(func() (int, int) { return 0, 0 })
	RegisterQueue("dashboard_test", func() float64 { return 0 }, func() float64 { return 1 })
	AgentConnected("env-dash")
	AgentConnected("env-dash")
	AgentDisconnected("env-dash")
	AgentHandshakeRejected("token")
	AgentDropped("env-dash", "disconnect", 1)
	AgentKeepalive("env-dash", time.Millisecond)
	AgentRequestStarted("env-dash")(AgentOutcomeOK)
	AgentOfflineBatch("env-dash", "events", true, 1, 0, time.Now())
	newTestRouter().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/containers/x", nil))
	PermissionDenied("host.view")
	WorkflowRun("manual", "failed", time.Second)
	ScheduleFired("container_backup", time.Now())
	BackupJobFinished("restore", "failure", time.Second)
	CollectorSweep(time.Second)
	AuditChainVerified(false, 1)
	PluginLoadFailed("addon-test")
	AgentRPC("env-dash", "compose")(nil)
	observeDB("sqlite", "query", time.Now(), errors.New("boom"))
	ReconcilerPass(time.Second, false)
	ReconcilerObserved(ReconcilerStates{InSync: 1}, time.Now())
	ReconcilerDriftTransition(true)
	agentSpansRelayed.WithLabelValues("dropped").Add(0)

	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(scrape(t)))
	if err != nil {
		t.Fatal(err)
	}
	metricName := regexp.MustCompile(`\b(mcharbor_[a-z_]+|go_[a-z_]+)`)
	for _, panel := range dash.Panels {
		for _, target := range panel.Targets {
			for _, name := range metricName.FindAllString(target.Expr, -1) {
				if strings.HasPrefix(name, "go_sql_") {
					continue
				}
				family := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, "_bucket"), "_count"), "_sum")
				if _, ok := families[family]; !ok {
					t.Errorf("panel %q queries %s, which the scrape does not export", panel.Title, name)
				}
			}
		}
	}
}

func TestTracingOffByDefaultInjectsNothing(t *testing.T) {
	shutdown, err := SetupTracing(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if TracingEnabled() {
		t.Fatal("an empty endpoint must leave tracing off")
	}
	headers := map[string]string{}
	_, end := StartAgentSpan(context.Background(), "env", http.MethodGet, "/containers/json", headers)
	end(nil)
	if len(headers) != 0 {
		t.Fatalf("disabled tracing wrote headers: %v", headers)
	}
}

func TestAgentSpanPropagatesTraceparentAcrossTheHop(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	prevProvider, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tracingEnabled.Store(true)
	t.Cleanup(func() {
		tracingEnabled.Store(false)
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevProp)
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("provider shutdown: %v", err)
		}
	})

	headers := map[string]string{}
	ctx, end := StartAgentSpan(context.Background(), "env-trace", http.MethodGet, "/containers/json", headers)
	defer end(nil)

	traceparent := headers["traceparent"]
	if traceparent == "" {
		t.Fatal("the agent envelope headers carry no traceparent")
	}
	carried := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier(headers))
	if got, want := trace.SpanContextFromContext(carried).TraceID(), trace.SpanContextFromContext(ctx).TraceID(); got != want {
		t.Fatalf("trace id across the hop = %s, want %s", got, want)
	}
}

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status %d", rec.Code)
	}
	return rec.Body.String()
}

func TestAgentRPCClassifiesOutcomes(t *testing.T) {
	cases := map[string]error{
		AgentOutcomeOK:        nil,
		AgentOutcomeTimeout:   fmt.Errorf("compose: %w", context.DeadlineExceeded),
		AgentOutcomeCancelled: context.Canceled,
		AgentOutcomeError:     errors.New("agent said no"),
	}
	for outcome, err := range cases {
		before := testutil.ToFloat64(agentRPCs.WithLabelValues("env-rpc", "compose", outcome))
		AgentRPC("env-rpc", "compose")(err)
		if got := testutil.ToFloat64(agentRPCs.WithLabelValues("env-rpc", "compose", outcome)) - before; got != 1 {
			t.Errorf("%v: counted %v under %s, want 1", err, got, outcome)
		}
	}
}

func TestReconcilerLagRisesFromTheStalestCheck(t *testing.T) {
	t.Cleanup(func() { SetReconcilerOldestCheck(time.Time{}) })
	ReconcilerObserved(ReconcilerStates{InSync: 1, Unknown: 1}, time.Now().Add(-90*time.Second))
	lag := gaugeValue(t, "mcharbor_reconciler_lag_seconds")
	if lag < 89 || lag > 120 {
		t.Fatalf("lag = %v, want about 90s", lag)
	}
	SetReconcilerOldestCheck(time.Time{})
	if lag := gaugeValue(t, "mcharbor_reconciler_lag_seconds"); lag != 0 {
		t.Fatalf("a node with nothing to check reports lag %v", lag)
	}
}

func gaugeValue(t *testing.T, name string) float64 {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("%s not exported", name)
	return 0
}
