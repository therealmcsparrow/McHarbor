// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TestSeatsAreActiveEnvironments pins the licence unit: one seat per active environment,
// agent-backed or direct. The agent behind an environment and the instance itself used to be
// counted on top of it, so one agent environment cost two seats.
func TestSeatsAreActiveEnvironments(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "seats.db")+"?_pragma=foreign_keys(0)")
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
	ctx := context.Background()
	svc := NewService(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), "test")

	before, err := svc.count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, kind string
		active   int
	}{
		{"env-socket", "socket", 1},
		{"env-ssh", "ssh", 1},
		{"env-agent", "agent", 1},
		{"env-agent-off", "agent", 0},
	} {
		if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type, is_active) VALUES (?, ?, ?, ?)`,
			row.id, row.id, row.kind, row.active); err != nil {
			t.Fatal(err)
		}
	}

	after, err := svc.count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.servers - before.servers; got != 3 {
		t.Errorf("seats added = %d, want 3 (two direct and one agent environment; the inactive one is free)", got)
	}
	if got := after.agents - before.agents; got != 1 {
		t.Errorf("agents added = %d, want 1", got)
	}
}
