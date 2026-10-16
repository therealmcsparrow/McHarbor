// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/fleetfixture"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
)

// scale divides every simulated latency so the run takes seconds, not minutes.
// Every bounded fan-out's wall time is linear in the per-call latency, so the
// reported figures are multiplied back up by the same factor.
const scale = 20

const fleetContainers = 300

func openEnvDB(t *testing.T, agentEnvs ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { closeutil.Report(db.Close(), "db") })
	for _, ddl := range []string{
		`CREATE TABLE environments (id TEXT PRIMARY KEY, connection_type TEXT)`,
		`CREATE TABLE container_stack_links (container_id TEXT, environment_id TEXT, stack_name TEXT, service_name TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	for _, id := range agentEnvs {
		if _, err := db.Exec(`INSERT INTO environments (id, connection_type) VALUES (?, 'agent')`, id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func fixturePool(t *testing.T, db *sql.DB, envID string, d *fleetfixture.Daemon) *docker.ClientPool {
	t.Helper()
	pool := docker.NewClientPool(db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c, err := d.Client()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetClient(envID, c)
	return pool
}

// TestFleetMeasure produces Phase 1's numbers. It is a measurement, not a
// guard, so it runs only when asked: MCHARBOR_FLEET_MEASURE=1.
func TestFleetMeasure(t *testing.T) {
	if os.Getenv("MCHARBOR_FLEET_MEASURE") != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}

	t.Run("collector pull, agent environment", func(t *testing.T) {
		d := fleetfixture.Start(fleetfixture.Config{Containers: fleetContainers, StatsLatency: fleetfixture.AgentStatsLatency / scale})
		defer d.Close()
		db := openEnvDB(t, "agent-env")
		svc := metrics.NewService(fixturePool(t, db, "agent-env", d))
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second/scale)
		defer cancel()
		start := time.Now()
		got, err := svc.AllContainerStats(ctx, "agent-env")
		elapsed := time.Since(start) * scale
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("AllContainerStats agent (pull fallback): %d/%d samples inside the 60s collector deadline, max in flight %d, wall %s",
			len(got), fleetContainers, d.MaxInFlight(), elapsed.Round(100*time.Millisecond))
	})

	t.Run("collector pull, local environment", func(t *testing.T) {
		d := fleetfixture.Start(fleetfixture.Config{Containers: fleetContainers, StatsLatency: fleetfixture.LocalStatsLatency / scale})
		defer d.Close()
		db := openEnvDB(t)
		svc := metrics.NewService(fixturePool(t, db, "local-env", d))
		start := time.Now()
		got, err := svc.AllContainerStats(context.Background(), "local-env")
		elapsed := time.Since(start) * scale
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("AllContainerStats local: %d samples, %d stats calls, max in flight %d, wall %s (collector deadline 20s)",
			len(got), d.StatsCalls.Load(), d.MaxInFlight(), elapsed.Round(100*time.Millisecond))
	})

	for _, tabs := range []int{1, 6, 20} {
		t.Run("containers page summary poll", func(t *testing.T) {
			d := fleetfixture.Start(fleetfixture.Config{Containers: fleetContainers, StatsLatency: fleetfixture.AgentStatsLatency / scale})
			defer d.Close()
			db := openEnvDB(t, "agent-env")
			svc := containers.NewService(fixturePool(t, db, "agent-env", d), db, t.TempDir(), nil)
			start := time.Now()
			var wg sync.WaitGroup
			var mu sync.Mutex
			minRows := fleetContainers
			for range tabs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					got, err := svc.BulkStats(context.Background(), "agent-env")
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					minRows = min(minRows, len(got))
					mu.Unlock()
				}()
			}
			wg.Wait()
			elapsed := time.Since(start) * scale
			t.Logf("BulkStats, %d tab(s) polling one agent env: %d stats calls reached the daemon per 5s poll (%d attempted), max in flight %d, worst tab got %d/%d rows, wall %s",
				tabs, d.StatsCalls.Load(), tabs*fleetContainers, d.MaxInFlight(), minRows, fleetContainers, elapsed.Round(100*time.Millisecond))
		})
	}

	t.Run("containers page through the stats hub", func(t *testing.T) {
		d := fleetfixture.Start(fleetfixture.Config{Containers: fleetContainers, StatsLatency: fleetfixture.AgentStatsLatency / scale})
		defer d.Close()
		db := openEnvDB(t, "agent-env")
		pool := fixturePool(t, db, "agent-env", d)
		hub := metrics.NewStatsHub(db, pool)
		defer hub.Close()
		svc := containers.NewService(pool, db, t.TempDir(), nil)
		svc.SetStatsHub(hub)
		const tabs = 20
		for poll := range 3 {
			var wg sync.WaitGroup
			var mu sync.Mutex
			minRows := fleetContainers
			start := time.Now()
			for range tabs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					got, err := svc.BulkStats(context.Background(), "agent-env")
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					minRows = min(minRows, len(got))
					mu.Unlock()
				}()
			}
			wg.Wait()
			t.Logf("hub poll %d, %d tabs: %d stats calls reached the daemon in total, max in flight %d, worst tab got %d/%d rows, answered in %s",
				poll+1, tabs, d.StatsCalls.Load(), d.MaxInFlight(), minRows, fleetContainers, (time.Since(start) * scale).Round(100*time.Millisecond))
		}
	})

	t.Run("container list payload", func(t *testing.T) {
		d := fleetfixture.Start(fleetfixture.Config{Containers: fleetContainers})
		defer d.Close()
		db := openEnvDB(t)
		svc := containers.NewService(fixturePool(t, db, "local-env", d), db, t.TempDir(), nil)
		list, err := svc.List(context.Background(), "local-env", true)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("GET /containers for %d containers: %d bytes of JSON, refetched every 10s per tab", len(list), len(body))
	})
}
