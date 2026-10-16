// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/fleetfixture"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/events"
)

// TestFleetMeasureSSE measures what open browser tabs cost on the event
// stream every tab holds (AppLayout → /api/events/stream).
func TestFleetMeasureSSE(t *testing.T) {
	if os.Getenv("MCHARBOR_FLEET_MEASURE") != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}
	const eventsPerRun = 50

	for _, tabs := range []int{1, 20, 120} {
		d := fleetfixture.Start(fleetfixture.Config{Containers: 10})
		db := openEnvDB(t, "agent-env")
		pool := fixturePool(t, db, "agent-env", d)
		h := events.NewHandler(&router.AppDeps{DockerPool: pool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		srv := httptest.NewServer(http.HandlerFunc(h.HandleStream))

		baseGoroutines := runtime.NumGoroutine()
		ctx, cancel := context.WithCancel(context.Background())
		var received atomic.Int64
		var connected sync.WaitGroup
		var done sync.WaitGroup
		start := time.Now()
		for range tabs {
			connected.Add(1)
			done.Add(1)
			go func() {
				defer done.Done()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?env=agent-env", nil)
				if err != nil {
					connected.Done()
					return
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					connected.Done()
					return
				}
				defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
				sc := bufio.NewScanner(resp.Body)
				signalled := false
				for sc.Scan() {
					line := sc.Text()
					if !signalled && strings.HasPrefix(line, "event: connected") {
						signalled = true
						connected.Done()
					}
					if strings.HasPrefix(line, "event: container") {
						received.Add(1)
					}
				}
				if !signalled {
					connected.Done()
				}
			}()
		}
		connected.Wait()
		connectTime := time.Since(start)
		subs := d.EventStreams.Load()
		goroutines := runtime.NumGoroutine() - baseGoroutines

		emitStart := time.Now()
		for i := range eventsPerRun {
			d.Emit("start", i%10)
		}
		want := int64(tabs * eventsPerRun)
		for received.Load() < want && time.Since(emitStart) < 10*time.Second {
			time.Sleep(5 * time.Millisecond)
		}
		fanOut := time.Since(emitStart)

		t.Logf("%3d tab(s): %3d Docker event subscriptions on the daemon, %4d daemon writes and %4d SSE frames for %d events, +%d goroutines, all connected in %s, fanned out in %s",
			tabs, subs, d.EventsSent.Load(), received.Load(), eventsPerRun, goroutines,
			connectTime.Round(time.Millisecond), fanOut.Round(time.Millisecond))

		cancel()
		done.Wait()
		srv.Close()
		d.Close()
	}
}
