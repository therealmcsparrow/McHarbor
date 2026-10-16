// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package fleetfixture is a synthetic Docker daemon for measuring McHarbor at
// fleet scale: hundreds of containers across a dozen environments, with the
// stats latency of a real daemon or of the remote agent, and counters that show
// how much work each McHarbor code path actually puts on the daemon.
package fleetfixture

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moby/moby/client"
)

// LocalStatsLatency is what a local daemon takes to answer a stream=false stats
// call: it samples twice, one second apart, so precpu_stats is populated.
const LocalStatsLatency = time.Second

// AgentStatsLatency is the measured cost of the same call through the remote
// agent, which the project's remote-agent constraints record as about two seconds.
const AgentStatsLatency = 2 * time.Second

// Config shapes one synthetic daemon.
type Config struct {
	Containers   int
	StatsLatency time.Duration
	// StreamInterval is the gap between documents on a stream=true stats call;
	// a real daemon emits one per second.
	StreamInterval time.Duration
}

// Daemon is one running synthetic daemon and the counters its handlers keep.
type Daemon struct {
	Server     *httptest.Server
	cfg        Config
	containers []map[string]any

	StatsCalls  atomic.Int64
	ListCalls   atomic.Int64
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	StreamsOpen atomic.Int64

	// EventStreams counts open /events subscriptions; eventSubs fans Emit out
	// to each of them the way the daemon does, one copy per subscriber.
	EventStreams atomic.Int64
	EventsSent   atomic.Int64
	eventMu      sync.Mutex
	eventSubs    map[chan []byte]struct{}
}

var (
	reList   = regexp.MustCompile(`^(/v[0-9.]+)?/containers/json$`)
	reStats  = regexp.MustCompile(`^(/v[0-9.]+)?/containers/([^/]+)/stats$`)
	reEvents = regexp.MustCompile(`^(/v[0-9.]+)?/events$`)
)

// Start launches a daemon with cfg on a loopback listener. Close it with
// Daemon.Close when the measurement is done.
func Start(cfg Config) *Daemon {
	if cfg.StreamInterval <= 0 {
		cfg.StreamInterval = time.Second
	}
	d := &Daemon{cfg: cfg, containers: Containers(cfg.Containers), eventSubs: make(map[chan []byte]struct{})}
	d.Server = httptest.NewServer(http.HandlerFunc(d.serve))
	return d
}

// Close stops the listener and any open stats streams.
func (d *Daemon) Close() { d.Server.Close() }

// MaxInFlight reports the highest number of stats calls the daemon was serving
// at once, which is the load a fan-out puts on a shared agent socket.
func (d *Daemon) MaxInFlight() int64 { return d.maxInFlight.Load() }

// ResetCounters zeroes every counter between measurement runs.
func (d *Daemon) ResetCounters() {
	d.StatsCalls.Store(0)
	d.ListCalls.Store(0)
	d.maxInFlight.Store(0)
}

// Client returns a Docker SDK client pointed at this daemon.
func (d *Daemon) Client() (*client.Client, error) {
	c, err := client.New(
		client.WithHost("tcp://"+strings.TrimPrefix(d.Server.URL, "http://")),
		client.WithAPIVersion("1.47"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating fixture client: %w", err)
	}
	return c, nil
}

func (d *Daemon) serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/_ping"):
		w.Header().Set("API-Version", "1.47")
		if _, err := w.Write([]byte("OK")); err != nil {
			return
		}
	case reList.MatchString(path):
		d.ListCalls.Add(1)
		writeJSON(w, d.containers)
	case reEvents.MatchString(path):
		d.serveEvents(w, r)
	case reStats.MatchString(path):
		id := reStats.FindStringSubmatch(path)[2]
		d.serveStats(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (d *Daemon) serveStats(w http.ResponseWriter, r *http.Request, id string) {
	d.StatsCalls.Add(1)
	stream := r.URL.Query().Get("stream")
	if stream == "0" || stream == "false" {
		now := d.inFlight.Add(1)
		defer d.inFlight.Add(-1)
		for {
			prev := d.maxInFlight.Load()
			if now <= prev || d.maxInFlight.CompareAndSwap(prev, now) {
				break
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(d.cfg.StatsLatency):
		}
		writeJSON(w, StatsDocument(id, 1))
		return
	}

	d.StreamsOpen.Add(1)
	defer d.StreamsOpen.Add(-1)
	w.Header().Set("Content-Type", "application/json")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	ticker := time.NewTicker(d.cfg.StreamInterval)
	defer ticker.Stop()
	for tick := int64(1); ; tick++ {
		if err := enc.Encode(StatsDocument(id, tick)); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// Emit sends one container event to every open /events subscription and
// reports how many copies the daemon wrote.
func (d *Daemon) Emit(action string, i int) int {
	doc, err := json.Marshal(map[string]any{
		"Type": "container", "Action": action, "status": action, "id": ContainerID(i),
		"Actor":    map[string]any{"ID": ContainerID(i), "Attributes": map[string]string{"name": fmt.Sprintf("c-%d", i)}},
		"time":     time.Now().Unix(),
		"timeNano": time.Now().UnixNano(),
	})
	if err != nil {
		return 0
	}
	d.eventMu.Lock()
	defer d.eventMu.Unlock()
	for ch := range d.eventSubs {
		select {
		case ch <- doc:
			d.EventsSent.Add(1)
		default:
		}
	}
	return len(d.eventSubs)
}

func (d *Daemon) serveEvents(w http.ResponseWriter, r *http.Request) {
	ch := make(chan []byte, 64)
	d.eventMu.Lock()
	d.eventSubs[ch] = struct{}{}
	d.eventMu.Unlock()
	d.EventStreams.Add(1)
	defer func() {
		d.eventMu.Lock()
		delete(d.eventSubs, ch)
		d.eventMu.Unlock()
		d.EventStreams.Add(-1)
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case doc := <-ch:
			if _, err := w.Write(append(doc, '\n')); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// Containers builds n running containers shaped like a Compose-managed fleet:
// each carries the labels, ports and mounts that make a real list response the
// size it is.
func Containers(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range n {
		project := fmt.Sprintf("stack-%02d", i/8)
		service := fmt.Sprintf("svc-%d", i%8)
		out[i] = map[string]any{
			"Id":      ContainerID(i),
			"Names":   []string{fmt.Sprintf("/%s-%s-1", project, service)},
			"Image":   fmt.Sprintf("registry.example.com/team/%s:1.%d.0", service, i%5),
			"ImageID": "sha256:" + strings.Repeat(fmt.Sprintf("%x", i%16), 64),
			"Command": "/docker-entrypoint.sh run --config /etc/app/config.yaml",
			"Created": time.Now().Add(-time.Duration(i) * time.Hour).Unix(),
			"State":   "running",
			"Status":  "Up 2 hours (healthy)",
			"Ports": []map[string]any{
				{"IP": "0.0.0.0", "PrivatePort": 8080, "PublicPort": 20000 + i, "Type": "tcp"},
			},
			"Labels": map[string]string{
				"com.docker.compose.project":          project,
				"com.docker.compose.service":          service,
				"com.docker.compose.container-number": "1",
				"com.docker.compose.config-hash":      strings.Repeat("a", 64),
				"com.docker.compose.version":          "2.29.1",
				"org.opencontainers.image.source":     "https://example.com/team/" + service,
			},
			"Mounts": []map[string]any{
				{"Type": "volume", "Name": project + "_data", "Destination": "/data", "RW": true},
			},
			"NetworkSettings": map[string]any{
				"Networks": map[string]any{
					project + "_default": map[string]any{"IPAddress": fmt.Sprintf("172.20.%d.%d", i/250, i%250+2)},
				},
			},
		}
	}
	return out
}

// ContainerID is the deterministic 64-hex id of fixture container i.
func ContainerID(i int) string {
	return fmt.Sprintf("%064x", i+1)
}

// StatsDocument is one daemon stats document for id; tick advances the CPU
// counters so consecutive stream documents yield a non-zero CPU percentage.
func StatsDocument(id string, tick int64) map[string]any {
	total := uint64(tick) * 50_000_000
	system := uint64(tick) * 1_000_000_000
	return map[string]any{
		"id":   id,
		"read": time.Now().UTC().Format(time.RFC3339Nano),
		"cpu_stats": map[string]any{
			"cpu_usage":        map[string]any{"total_usage": total + 50_000_000},
			"system_cpu_usage": system + 1_000_000_000,
			"online_cpus":      4,
		},
		"precpu_stats": map[string]any{
			"cpu_usage":        map[string]any{"total_usage": total},
			"system_cpu_usage": system,
			"online_cpus":      4,
		},
		"memory_stats": map[string]any{"usage": 128 << 20, "limit": 2 << 30},
		"pids_stats":   map[string]any{"current": 12},
		"networks": map[string]any{
			"eth0": map[string]any{"rx_bytes": tick * 1024, "tx_bytes": tick * 512},
		},
		"blkio_stats": map[string]any{
			"io_service_bytes_recursive": []map[string]any{
				{"op": "read", "value": tick * 4096},
				{"op": "write", "value": tick * 2048},
			},
		},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("fleet fixture: encoding a daemon reply failed", "error", err)
		http.Error(w, "fleet fixture: could not encode the reply", http.StatusInternalServerError)
	}
}
