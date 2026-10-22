// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"

	_ "modernc.org/sqlite"
)

func TestMergeEnvironmentsRunsTheMergedHookOnlyAfterACommit(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "merge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, id := range []string{"src", "tgt"} {
		mustExec(t, database, `INSERT INTO environments (id, name) VALUES (?, ?)`, id, id)
	}
	svc := NewService(database, docker.NewClientPool(database, nil, slog.Default()), nil, nil)

	var calls [][2]string
	SetMergedHook(func(src, tgt string) { calls = append(calls, [2]string{src, tgt}) })
	t.Cleanup(func() { SetMergedHook(nil) })

	if _, err := svc.MergeEnvironments(context.Background(), "src", "tgt", true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("a dry run called the hook: %v", calls)
	}
	if _, err := svc.MergeEnvironments(context.Background(), "src", "tgt", false); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(calls) != 1 || calls[0] != [2]string{"src", "tgt"} {
		t.Fatalf("hook calls = %v, want one call (src, tgt)", calls)
	}
}
