// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// envLiveURL is the base URL of a running instance to measure, e.g.
	// https://localhost:8705. Unset skips the live SSE measurement.
	envLiveURL = "MCHARBOR_FLEET_LIVE_URL"
	// envLiveToken is an API key sent as a Bearer token.
	envLiveToken = "MCHARBOR_FLEET_LIVE_TOKEN"
	// envLiveEnv names the environment to subscribe to, by id or name.
	envLiveEnv = "MCHARBOR_FLEET_LIVE_ENV"
	// envLiveSteps is a comma-separated list of concurrent stream counts.
	envLiveSteps = "MCHARBOR_FLEET_LIVE_STEPS"
	// envLiveHold is how long each step holds its streams open, as a Go duration.
	envLiveHold = "MCHARBOR_FLEET_LIVE_HOLD"
	// envLiveContainer names the instance's container for `docker stats`.
	envLiveContainer = "MCHARBOR_FLEET_LIVE_CONTAINER"
	// envLiveInsecure=1 accepts the instance's self-signed certificate.
	envLiveInsecure = "MCHARBOR_FLEET_LIVE_INSECURE"
)

// liveClient issues only GET requests against the instance: the measurement
// must never be able to change the instance or the host behind the agent.
type liveClient struct {
	base, token string
	http        *http.Client
}

func (c *liveClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req)
}

// TestFleetMeasureSSELive opens 1, ~20 and N concurrent /api/events/stream
// subscriptions against a running instance's agent environment and reports
// connect latency, delivery, agent round trips and the container's CPU and memory.
func TestFleetMeasureSSELive(t *testing.T) {
	if os.Getenv(envMeasure) != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(envLiveURL)), "/")
	token := strings.TrimSpace(os.Getenv(envLiveToken))
	if base == "" || token == "" {
		t.Skipf("set %s and %s to measure a running instance", envLiveURL, envLiveToken)
	}
	steps := []int{1, 20, 100}
	if s := os.Getenv(envLiveSteps); s != "" {
		steps = nil
		for _, f := range strings.Split(s, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil || n < 1 {
				t.Fatalf("%s=%q: %v", envLiveSteps, s, err)
			}
			steps = append(steps, n)
		}
	}
	hold := time.Minute
	if s := os.Getenv(envLiveHold); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("%s=%q: %v", envLiveHold, s, err)
		}
		hold = d
	}

	tr := &http.Transport{ForceAttemptHTTP2: true, MaxIdleConnsPerHost: 512}
	if os.Getenv(envLiveInsecure) == "1" {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	c := &liveClient{base: base, token: token, http: &http.Client{Transport: tr}}
	envID, envLabel := resolveLiveEnv(t, c, os.Getenv(envLiveEnv))
	t.Logf("environment %s", envLabel)

	// The instance talks to the agent on its own (collectors, other tabs), so
	// an idle window of the same length is the noise floor for every delta below.
	idleStart := scrape(t, c, envID)
	time.Sleep(hold)
	idleEnd := scrape(t, c, envID)
	t.Logf("idle baseline over %s: agent requests +%.0f, in flight %.0f, goroutines %.0f, docker stats [%s]",
		hold, idleEnd.requests-idleStart.requests, idleEnd.inFlight, idleEnd.goroutines, dockerStats(t))

	var rows []string
	for _, n := range steps {
		row := runLiveStep(t, c, envID, n, hold)
		t.Log(row)
		rows = append(rows, row)
		// Let the broker tear down and the transport settle before the next step.
		time.Sleep(5 * time.Second)
	}
	t.Log("SSE fan-out over the live agent link\n" + strings.Join(rows, "\n"))
}

// resolveLiveEnv finds the environment by id or name through the list endpoint.
func resolveLiveEnv(t *testing.T, c *liveClient, want string) (string, string) {
	t.Helper()
	if want == "" {
		t.Fatalf("set %s to the environment id or name to subscribe to", envLiveEnv)
	}
	resp, err := c.get(context.Background(), "/api/environments")
	if err != nil {
		t.Fatalf("listing environments: %v", err)
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	var body struct {
		Data []struct {
			ID, Name, ConnectionType, AgentInstallKind, AgentBuild, AgentStatus string
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding environments: %v", err)
	}
	for _, e := range body.Data {
		if e.ID == want || strings.EqualFold(e.Name, want) {
			return e.ID, fmt.Sprintf("%s (%s): %s, install %s, build %s, %s",
				e.Name, e.ID, e.ConnectionType, e.AgentInstallKind, e.AgentBuild, e.AgentStatus)
		}
	}
	t.Fatalf("no environment %q", want)
	return "", ""
}

// subscriber is one simulated tab.
type subscriber struct {
	cancel    context.CancelFunc
	connected time.Duration
	proto     string
	status    int
	err       error
	ended     atomic.Bool // the stream ended without the tab asking
	errFrames atomic.Int64
	mu        sync.Mutex
	events    map[string]time.Time
}

// runLiveStep opens n streams, holds them, disturbs them the way closing tabs
// does, and returns one formatted result row.
func runLiveStep(t *testing.T, c *liveClient, envID string, n int, hold time.Duration) string {
	t.Helper()
	statsBefore := dockerStats(t)
	mBefore := scrape(t, c, envID)

	subs := make([]*subscriber, n)
	var connected sync.WaitGroup
	var done sync.WaitGroup
	start := time.Now()
	for i := range n {
		s := &subscriber{events: map[string]time.Time{}}
		subs[i] = s
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		connected.Add(1)
		done.Add(1)
		go s.run(ctx, c, envID, start, &connected, &done)
	}
	// A tab closing while the others are still loading.
	midLoad := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		resp, err := c.get(ctx, "/api/events/stream?env="+envID)
		if err == nil {
			closeutil.Report(resp.Body.Close(), "resp.Body")
		}
		midLoad <- err
	}()
	connected.Wait()
	connectWall := time.Since(start)
	t.Logf("%d streams open at %s UTC, holding until %s UTC", n,
		time.Now().UTC().Format("15:04:05"), time.Now().Add(hold).UTC().Format("15:04:05"))
	midLoadResult := <-midLoad
	mConnected := scrape(t, c, envID)
	statsConnected := dockerStats(t)

	// Close the first tab (the one whose request started the shared stream),
	// then one more, while the rest keep listening.
	closed := map[int]bool{}
	time.Sleep(hold / 3)
	if n > 1 {
		subs[0].cancel()
		closed[0] = true
	}
	time.Sleep(hold / 3)
	if n > 2 {
		subs[n/2].cancel()
		closed[n/2] = true
	}
	time.Sleep(hold - 2*(hold/3))
	statsHeld := dockerStats(t)
	mHeld := scrape(t, c, envID)

	for _, s := range subs {
		s.cancel()
	}
	done.Wait()
	time.Sleep(3 * time.Second)
	mAfter := scrape(t, c, envID)

	var conn []time.Duration
	var failed, unexpectedEnds int
	var errFrames int64
	protos := map[string]int{}
	union := map[string]time.Time{}
	var latencies []time.Duration
	for i, s := range subs {
		if s.err != nil || s.status != http.StatusOK {
			failed++
			continue
		}
		conn = append(conn, s.connected)
		protos[s.proto]++
		if s.ended.Load() && !closed[i] {
			unexpectedEnds++
		}
		errFrames += s.errFrames.Load()
		s.mu.Lock()
		for k, at := range s.events {
			if first, ok := union[k]; !ok || at.Before(first) {
				union[k] = at
			}
			if sec, err := strconv.ParseInt(k[strings.LastIndex(k, "|")+1:], 10, 64); err == nil {
				latencies = append(latencies, at.Sub(time.Unix(sec, 0)))
			}
		}
		s.mu.Unlock()
	}
	// Every surviving tab should hold every event seen after the last close;
	// before that, the closed tabs legitimately miss what came later.
	var frames, missing int
	for i, s := range subs {
		if s.err != nil || s.status != http.StatusOK || closed[i] {
			continue
		}
		s.mu.Lock()
		frames += len(s.events)
		for k := range union {
			if _, ok := s.events[k]; !ok {
				missing++
			}
		}
		s.mu.Unlock()
	}

	sort.Slice(conn, func(i, j int) bool { return conn[i] < conn[j] })
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return fmt.Sprintf("%4d streams (%v): connect p50 %s p95 %s max %s (wall %s), failed %d; mid-load close err=%v; "+
		"agent requests +%.0f during connect, +%.0f during hold, +%.0f after close, in flight %.0f/%.0f/%.0f, dropped +%.0f; "+
		"goroutines %.0f→%.0f→%.0f→%.0f; events %d, frames to survivors %d, missing %d, latency p50 %s max %s, error frames %d, unexpected ends %d; "+
		"docker stats before [%s] connected [%s] held [%s]",
		n, protos, pct(conn, 50), pct(conn, 95), pct(conn, 100), connectWall.Round(time.Millisecond), failed, midLoadResult,
		mConnected.requests-mBefore.requests, mHeld.requests-mConnected.requests, mAfter.requests-mHeld.requests,
		mBefore.inFlight, mConnected.inFlight, mAfter.inFlight, mAfter.dropped-mBefore.dropped,
		mBefore.goroutines, mConnected.goroutines, mHeld.goroutines, mAfter.goroutines,
		len(union), frames, missing, pct(latencies, 50), pct(latencies, 100), errFrames, unexpectedEnds,
		statsBefore, statsConnected, statsHeld)
}

func (s *subscriber) run(ctx context.Context, c *liveClient, envID string, start time.Time, connected, done *sync.WaitGroup) {
	defer done.Done()
	signalled := false
	signal := func() {
		if !signalled {
			signalled = true
			connected.Done()
		}
	}
	defer signal()
	resp, err := c.get(ctx, "/api/events/stream?env="+envID)
	if err != nil {
		s.err = err
		return
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	s.status, s.proto = resp.StatusCode, resp.Proto
	if resp.StatusCode != http.StatusOK {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			s.err = err
		}
		return
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			if event == "connected" {
				s.connected = time.Since(start)
				signal()
			}
			if event == "error" {
				s.errFrames.Add(1)
			}
		case strings.HasPrefix(line, "data: ") && event != "connected" && event != "error":
			var d struct {
				Type, Action string
				Actor        struct{ ID string }
				Time         int64
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &d) == nil {
				key := fmt.Sprintf("%s|%s|%s|%d", d.Type, d.Action, d.Actor.ID, d.Time)
				s.mu.Lock()
				if _, dup := s.events[key]; !dup {
					s.events[key] = time.Now()
				}
				s.mu.Unlock()
			}
		}
	}
	if ctx.Err() == nil {
		s.ended.Store(true)
	}
}

// liveMetrics is the slice of the instance's Prometheus exposition this
// measurement reads: agent traffic for one environment and the goroutine count.
type liveMetrics struct {
	requests, inFlight, dropped, goroutines float64
}

// scrape reads /api/metrics, which serves memory-only counters and makes no
// Docker, agent or database call, so reading it does not disturb the link.
func scrape(t *testing.T, c *liveClient, envID string) liveMetrics {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := c.get(ctx, "/api/metrics/")
	if err != nil {
		t.Logf("scrape: %v", err)
		return liveMetrics{}
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	var m liveMetrics
	envLabel := `environment="` + envID + `"`
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, val := metricLine(line)
		switch {
		case name == "go_goroutines":
			m.goroutines += val
		case name == "mcharbor_agent_requests_total" && strings.Contains(line, envLabel):
			m.requests += val
		case name == "mcharbor_agent_requests_in_flight" && strings.Contains(line, envLabel):
			m.inFlight += val
		case name == "mcharbor_agent_dropped_messages_total" && strings.Contains(line, envLabel):
			m.dropped += val
		}
	}
	return m
}

func metricLine(line string) (string, float64) {
	sp := strings.LastIndexByte(line, ' ')
	if sp < 0 {
		return "", 0
	}
	v, err := strconv.ParseFloat(line[sp+1:], 64)
	if err != nil {
		return "", 0
	}
	name := line[:sp]
	if i := strings.IndexByte(name, '{'); i >= 0 {
		name = name[:i]
	}
	return name, v
}

// dockerStats samples the instance container's CPU and memory, or returns ""
// when no container was named.
func dockerStats(t *testing.T) string {
	t.Helper()
	name := os.Getenv(envLiveContainer)
	if name == "" {
		return ""
	}
	out, err := exec.Command("docker", "stats", "--no-stream", "--format", "{{.CPUPerc}} {{.MemUsage}}", name).Output()
	if err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	i := (len(d) - 1) * p / 100
	return d[i].Round(time.Millisecond)
}
