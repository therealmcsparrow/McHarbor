// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

func migrated(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(db.Config{Driver: db.DriverSQLite, Path: filepath.Join(t.TempDir(), "r.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestRegistryTopology guards what failover depends on: a beating node is alive, a
// silent one is not, a clean stop is recorded, and two live actives are reported.
func TestRegistryTopology(t *testing.T) {
	database := migrated(t)
	ctx := context.Background()
	clock := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	a := NewRegistry(database, "a", "http://a:8705", func() int { return 2 }, quiet())
	b := NewRegistry(database, "b", "", nil, quiet())
	for _, r := range []*Registry{a, b} {
		r.now = func() time.Time { return clock }
	}
	a.SetRole(RoleActive)
	b.SetRole(RoleActive)
	if err := a.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := a.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if !list[0].Self || list[0].Agents != 2 || !list[0].Alive || list[1].Self {
		t.Fatalf("a = %+v", list[0])
	}
	if got := ActiveConflict(list); len(got) != 2 {
		t.Fatalf("two live actives must be a conflict: %v", got)
	}

	clock = clock.Add(aliveWithin + time.Second)
	if err := a.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	list, err = a.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !list[0].Alive || list[1].Alive {
		t.Fatalf("b stopped beating and must not be alive: %+v", list)
	}
	if got := ActiveConflict(list); got != nil {
		t.Fatalf("a dead active is no conflict: %v", got)
	}

	a.SetState(ctx, StateDraining)
	list, err = b.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].State != StateDraining {
		t.Fatalf("draining must be published at once: %+v", list[0])
	}
}

// TestReadyTracksDrainingAndStandby guards the load-balancer contract: a draining node
// or a standby answers not-ready even though its database is fine.
func TestReadyTracksDrainingAndStandby(t *testing.T) {
	database := migrated(t)
	coord, err := coordinator.New(database, "a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := coord.Close(); err != nil {
			t.Errorf("closing coord: %v", err)
		}
	})
	svc := NewService(database, coord, quiet(), nil)
	reg := NewRegistry(database, "a", "", nil, quiet())
	svc.SetRegistry(reg)
	ctx := context.Background()
	if ok, checks := svc.Ready(ctx); !ok {
		t.Fatalf("a running peer must be ready: %+v", checks)
	}
	reg.SetState(ctx, StateDraining)
	if ok, _ := svc.Ready(ctx); ok {
		t.Fatal("a draining node must not be ready")
	}
	reg.SetState(ctx, StateRunning)
	reg.SetRole(RoleStandby)
	if ok, _ := svc.Ready(ctx); ok {
		t.Fatal("a standby must not be ready")
	}
}
