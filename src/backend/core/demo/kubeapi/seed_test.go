// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSeedDBIdempotent(t *testing.T) {
	migrations, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "plugins", "Plugins", "addon-kubernetes", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Skip("addon-kubernetes migrations not present in this checkout")
	}
	sort.Strings(migrations)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("closing sqlite: %v", cerr)
		}
	})
	ctx := context.Background()
	for _, m := range migrations {
		raw, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("reading %s: %v", m, err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("applying %s: %v", m, err)
		}
	}

	clusters := New(1, testBase).Clusters()
	token := func(key string) string { return "demo-token-" + key }
	for i := 0; i < 2; i++ {
		if err := SeedDB(ctx, db, "http://127.0.0.1:18443", clusters, token); err != nil {
			t.Fatalf("SeedDB run %d: %v", i+1, err)
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT id, name, namespace, server_url, bearer_token, is_default, is_active FROM plugin_kubernetes_clusters ORDER BY is_default DESC, name`)
	if err != nil {
		t.Fatalf("querying clusters: %v", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			t.Errorf("closing rows: %v", cerr)
		}
	}()
	type row struct {
		id, name, ns, url, token string
		isDefault, isActive      int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name, &r.ns, &r.url, &r.token, &r.isDefault, &r.isActive); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("%d cluster rows after two seeds, want 3", len(got))
	}
	if got[0].name != "prod-eu" || got[0].isDefault != 1 || got[1].isDefault != 0 || got[2].isDefault != 0 {
		t.Fatalf("default flags wrong: %+v", got)
	}
	for _, r := range got {
		if r.url != "http://127.0.0.1:18443" || r.token != "demo-token-"+r.name || r.isActive != 1 || r.ns != "" || r.id != ClusterID(r.name) || len(r.id) != 20 {
			t.Fatalf("row wrong: %+v", r)
		}
	}
	if err := SeedDB(ctx, db, "http://127.0.0.1:19000", clusters, token); err != nil {
		t.Fatalf("re-seed with a new port: %v", err)
	}
	var url string
	if err := db.QueryRowContext(ctx, `SELECT server_url FROM plugin_kubernetes_clusters WHERE id = ?`, ClusterID("edge")).Scan(&url); err != nil {
		t.Fatalf("reading edge row: %v", err)
	}
	if url != "http://127.0.0.1:19000" {
		t.Fatalf("re-seed did not move server_url: %s", url)
	}
}
