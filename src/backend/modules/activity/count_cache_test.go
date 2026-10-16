// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func TestTheUnfilteredCountIsReusedBrieflyAndFilteredCountsNeverAre(t *testing.T) {
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	add := func(n int, env string) {
		for i := 0; i < n; i++ {
			if _, err := database.Exec(`INSERT INTO container_events (id, environment_id, container_id, event_type, action, timestamp)
				VALUES (?, ?, 'c1', 'container', 'start', ?)`, fmt.Sprintf("%s-%d-%d", env, n, i), env, time.Now().UTC().Format(time.RFC3339)); err != nil {
				t.Fatal(err)
			}
		}
	}
	clock := time.Now()
	svc := NewService(database)
	svc.now = func() time.Time { return clock }
	ctx := context.Background()

	add(3, "e1")
	if _, total, err := svc.List(ctx, 1, 25, ListFilter{}); err != nil || total != 3 {
		t.Fatalf("first count: %d %v", total, err)
	}
	add(2, "e1")
	if _, total, err := svc.List(ctx, 1, 25, ListFilter{}); err != nil || total != 3 {
		t.Fatalf("the unfiltered count was not reused within the window: %d %v", total, err)
	}
	if _, total, err := svc.List(ctx, 1, 25, ListFilter{EnvironmentID: "e1"}); err != nil || total != 5 {
		t.Fatalf("a filtered count must always be fresh: %d %v", total, err)
	}
	clock = clock.Add(31 * time.Second)
	if _, total, err := svc.List(ctx, 1, 25, ListFilter{}); err != nil || total != 5 {
		t.Fatalf("after the window the count must refresh: %d %v", total, err)
	}
}
