// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package telemetry instruments the McHarbor process itself for Prometheus and
// OpenTelemetry. Every metric is an in-memory counter, so a scrape never
// reaches Docker, an agent, or the database.
package telemetry

import (
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// namespace prefixes every family, so McHarbor's series never collide with a
// scraped exporter that happens to share a metric name.
const namespace = "mcharbor"

// Registry is a private registry rather than prometheus.DefaultRegisterer, so a
// transitive dependency that registers globally cannot widen what a scrape
// returns or add unbounded labels behind this package's back.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = newCounterVec("http_requests_total",
		"HTTP requests served, by chi route pattern, method and status class.",
		"route", "method", "code")
	httpDuration = newHistogramVec("http_request_duration_seconds",
		"HTTP request latency by chi route pattern; streams and upgrades are excluded.",
		prometheus.DefBuckets, "route", "method")
	httpInFlight = newGauge("http_requests_in_flight",
		"HTTP requests currently being served, streams included.")
	permissionDenials = newCounterVec("permission_denials_total",
		"Requests refused by a permission gate, by the permission key that was missing.",
		"permission")

	agentConnects = newCounterVec("agent_connects_total",
		"Agent connections accepted into the pool.", "environment")
	agentReconnects = newCounterVec("agent_reconnects_total",
		"Agent connections for an environment that had connected before in this process.", "environment")
	agentDisconnects = newCounterVec("agent_disconnects_total",
		"Agent connections that left the pool.", "environment")
	agentRejected = newCounterVec("agent_handshakes_rejected_total",
		"Agent handshakes refused before registration, by stage.", "stage")
	agentRoundTrip = newHistogramVec("agent_request_duration_seconds",
		"Round trip of a proxied Docker API request through an agent, to the response head.",
		[]float64{.01, .025, .05, .1, .25, .5, 1, 2, 5, 10, 30, 60}, "environment")
	agentRequests = newCounterVec("agent_requests_total",
		"Proxied Docker API requests through an agent, by outcome.", "environment", "outcome")
	agentInFlight = newGaugeVec("agent_requests_in_flight",
		"Proxied requests awaiting an agent response head.", "environment")
	agentLastResponse = newGaugeVec("agent_last_response_timestamp_seconds",
		"Unix time the agent last completed a proxied request; flat while requests time out means a wedged agent.",
		"environment")
	agentDropped = newCounterVec("agent_dropped_messages_total",
		"Agent messages or pending requests discarded, by reason.", "environment", "reason")
	agentKeepalive = newHistogramVec("agent_keepalive_latency_seconds",
		"Keepalive ping-to-pong round trip per agent.",
		[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2, 5}, "environment")
	agentRPCDuration = newHistogramVec("agent_rpc_duration_seconds",
		"Round trip of an agent RPC other than the Docker proxy (compose, host stats, prune, transfers, compute, snapshots), by operation.",
		[]float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 300, 900, 3600}, "environment", "op")
	agentRPCs = newCounterVec("agent_rpc_total",
		"Agent RPCs other than the Docker proxy, by operation and outcome.", "environment", "op", "outcome")
	agentOfflineBatches = newCounterVec("agent_offline_batches_total",
		"Offline replay batches received from agents, by kind and outcome.", "kind", "outcome")
	agentOfflineRows = newCounterVec("agent_offline_rows_total",
		"Rows ingested from offline replay batches.", "kind")
	agentOfflineRemaining = newGaugeVec("agent_offline_buffer_remaining",
		"Rows the agent reported still buffered after its last replay batch.", "environment", "kind")
	agentOfflineLag = newHistogramVec("agent_offline_replay_lag_seconds",
		"Age of the oldest row in a replay batch when the server ingested it.",
		[]float64{1, 10, 60, 300, 900, 3600, 4 * 3600, 12 * 3600, 24 * 3600, 72 * 3600}, "kind")

	workflowRuns = newCounterVec("workflow_runs_total",
		"Workflow runs recorded, by trigger and outcome.", "trigger", "outcome")
	workflowDuration = newHistogramVec("workflow_run_duration_seconds",
		"Workflow run duration by outcome.",
		[]float64{.1, .5, 1, 5, 15, 60, 300, 900, 3600}, "outcome")
	scheduleFires = newCounterVec("schedule_fires_total",
		"Schedules fired, by scheduler.", "scheduler")
	scheduleLateness = newHistogramVec("schedule_lateness_seconds",
		"Delay between a schedule's intended minute and the moment it fired.",
		[]float64{1, 5, 15, 30, 60, 120, 300, 900}, "scheduler")
	backupJobs = newHistogramVec("backup_job_duration_seconds",
		"Container backup and restore run durations, by operation and outcome.",
		[]float64{1, 5, 15, 60, 300, 900, 1800, 3600, 7200}, "operation", "outcome")
	collectorSweep = newHistogram("metrics_collector_sweep_duration_seconds",
		"Duration of one background container/host metrics sweep.",
		[]float64{.1, .5, 1, 2, 5, 10, 20, 30, 60})
	auditVerifyOK = newGauge("audit_chain_verify_ok",
		"1 when the last audit hash-chain verification succeeded, 0 when it failed or warned.")
	auditVerifyAt = newGauge("audit_chain_verify_timestamp_seconds",
		"Unix time of the last audit hash-chain verification.")
	auditVerifyRows = newGauge("audit_chain_verify_checked_rows",
		"Rows checked by the last audit hash-chain verification.")
	pluginLoadFailures = newCounterVec("plugin_load_failures_total",
		"Plugin load attempts that failed, by plugin slug.", "plugin")
)

var (
	queueMu    sync.Mutex
	queueNames = map[string]bool{}
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		&agentPoolCollector{},
		&pluginCollector{},
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "build_info",
			Help:        "Always 1; the labels carry the running version and build stamp.",
			ConstLabels: prometheus.Labels{"version": appversion.Current(), "build": appversion.Build()},
		}, func() float64 { return 1 }),
	)
}

// RegisterDB exposes database/sql pool saturation for db, labelled by backend
// driver; sql.DB.Stats is an in-memory read, so the scrape issues no query.
func RegisterDB(db *sql.DB, driver string) {
	if db == nil {
		return
	}
	if driver == "" {
		driver = "unknown"
	}
	Registry.MustRegister(collectors.NewDBStatsCollector(db, driver))
}

// RegisterQueue exposes a queue's current depth and capacity under name. A
// second registration of the same name is ignored so a restarted worker
// cannot panic the process on a duplicate collector.
func RegisterQueue(name string, depth, capacity func() float64) {
	queueMu.Lock()
	defer queueMu.Unlock()
	if queueNames[name] {
		return
	}
	queueNames[name] = true
	labels := prometheus.Labels{"queue": name}
	Registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: namespace, Name: "queue_depth",
			Help: "Items waiting in an internal work queue.", ConstLabels: labels}, depth),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: namespace, Name: "queue_capacity",
			Help: "Capacity of an internal work queue.", ConstLabels: labels}, capacity),
	)
}

// AgentPoolStats is the in-memory view of the agent pool a scrape reads.
type AgentPoolStats struct {
	Connected int
	Measuring int
	Stale     int
}

// RegisterAgentPool exposes connected agents by keepalive state through stats,
// which must answer from memory. A later call replaces the source rather than
// registering a second collector, which the registry would reject.
func RegisterAgentPool(stats func() AgentPoolStats) {
	agentPoolSource.Store(&stats)
}

// RegisterPlugins exposes how many plugins are loaded and how many failed, read
// from the runtime's in-memory table; like RegisterAgentPool it replaces the source.
func RegisterPlugins(counts func() (loaded, failed int)) {
	pluginSource.Store(&counts)
}

var (
	agentPoolSource atomic.Pointer[func() AgentPoolStats]
	pluginSource    atomic.Pointer[func() (int, int)]
)

var agentPoolDesc = prometheus.NewDesc(namespace+"_agents",
	"Agents in the connection pool, by keepalive state (live, measuring, stale).", []string{"state"}, nil)

type agentPoolCollector struct{}

// Describe implements prometheus.Collector.
func (c *agentPoolCollector) Describe(ch chan<- *prometheus.Desc) { ch <- agentPoolDesc }

// Collect implements prometheus.Collector.
func (c *agentPoolCollector) Collect(ch chan<- prometheus.Metric) {
	var s AgentPoolStats
	if fn := agentPoolSource.Load(); fn != nil {
		s = (*fn)()
	}
	live := s.Connected - s.Measuring - s.Stale
	if live < 0 {
		live = 0
	}
	ch <- prometheus.MustNewConstMetric(agentPoolDesc, prometheus.GaugeValue, float64(live), "live")
	ch <- prometheus.MustNewConstMetric(agentPoolDesc, prometheus.GaugeValue, float64(s.Measuring), "measuring")
	ch <- prometheus.MustNewConstMetric(agentPoolDesc, prometheus.GaugeValue, float64(s.Stale), "stale")
}

var pluginDesc = prometheus.NewDesc(namespace+"_plugins",
	"Plugins known to the runtime, by state (loaded, failed).", []string{"state"}, nil)

type pluginCollector struct{}

// Describe implements prometheus.Collector.
func (c *pluginCollector) Describe(ch chan<- *prometheus.Desc) { ch <- pluginDesc }

// Collect implements prometheus.Collector.
func (c *pluginCollector) Collect(ch chan<- prometheus.Metric) {
	var loaded, failed int
	if fn := pluginSource.Load(); fn != nil {
		loaded, failed = (*fn)()
	}
	ch <- prometheus.MustNewConstMetric(pluginDesc, prometheus.GaugeValue, float64(loaded), "loaded")
	ch <- prometheus.MustNewConstMetric(pluginDesc, prometheus.GaugeValue, float64(failed), "failed")
}

// boundedLabel folds a free-form value into allowed, so a caller passing an
// unexpected string can never mint a new series.
func boundedLabel(v string, allowed ...string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return "other"
}

func labelOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func newCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: name, Help: help}, labels)
	Registry.MustRegister(c)
	return c
}

func newGauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help})
	Registry.MustRegister(g)
	return g
}

func newGaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help}, labels)
	Registry.MustRegister(g)
	return g
}

func newHistogram(name, help string, buckets []float64) prometheus.Histogram {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: namespace, Name: name, Help: help, Buckets: buckets})
	Registry.MustRegister(h)
	return h
}

func newHistogramVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: namespace, Name: name, Help: help, Buckets: buckets}, labels)
	Registry.MustRegister(h)
	return h
}
