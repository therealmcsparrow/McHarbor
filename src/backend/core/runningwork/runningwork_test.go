// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runningwork

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

func TestCountReportsEachKind(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "rw.db")+"?_pragma=foreign_keys(0)")
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
	if _, err := database.Exec(`INSERT INTO container_backup_runs (id, environment_id, container_id, status, started_at, created_at, updated_at) VALUES ('r1', 'e', 'c', 'running', 't', 't', 't'), ('r2', 'e', 'c', 'success', 't', 't', 't')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := map[Kind]int{}
	for _, r := range Count(context.Background(), database) {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Kind, r.Err)
		}
		got[r.Kind] = r.Count
	}
	if got[ContainerBackups] != 1 || got[InstanceExports] != 0 {
		t.Fatalf("counts = %+v, want one running container backup and no export", got)
	}

	if _, err := database.Exec(`DROP TABLE instance_backup_exports`); err != nil {
		t.Fatal(err)
	}
	for _, r := range Count(context.Background(), database) {
		if r.Kind == InstanceExports && r.Err == nil {
			t.Fatal("an unreadable kind must carry its error, not a zero count")
		}
	}
}
