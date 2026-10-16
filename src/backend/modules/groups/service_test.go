// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package groups

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func TestListCapsAtOneThousandGroups(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "groups.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := 0; i < 1100; i++ {
		if _, err := database.ExecContext(context.Background(), `INSERT INTO groups (id, name, is_system) VALUES (?, ?, 0)`,
			fmt.Sprintf("g%04d", i), fmt.Sprintf("group-%04d", i)); err != nil {
			t.Fatal(err)
		}
	}
	items, err := NewService(database).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1000 {
		t.Fatalf("List returned %d groups, want the 1000 cap", len(items))
	}
}
