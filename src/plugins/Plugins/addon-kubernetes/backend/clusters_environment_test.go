// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	_ "modernc.org/sqlite"
)

type sqliteDB struct{ db *sql.DB }

func (h sqliteDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := h.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (h sqliteDB) Query(ctx context.Context, query string, args ...any) (plugin.Rows, error) {
	return h.db.QueryContext(ctx, query, args...)
}

func (sqliteDB) KV() plugin.KVStore { return nil }

// legacyStore is a store over a table created before environment_id existed, the
// shape every install that predates K7 has.
func legacyStore(t *testing.T) *clusterStore {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "k8s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE plugin_kubernetes_clusters (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, namespace TEXT NOT NULL DEFAULT '',
		server_url TEXT NOT NULL DEFAULT '', kubeconfig TEXT NOT NULL DEFAULT '',
		bearer_token TEXT NOT NULL DEFAULT '', ca_cert TEXT NOT NULL DEFAULT '',
		is_default INTEGER NOT NULL DEFAULT 0, is_active INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return &clusterStore{db: sqliteDB{db: db}, enc: fakeEncryption{}}
}

// fakeEncryption stands in for the host service: the store refuses to keep credentials
// without one, and the tests only need a reversible marker.
type fakeEncryption struct{}

func (fakeEncryption) EncryptString(plain string) (string, error) { return "enc:v1:" + plain, nil }

func (fakeEncryption) DecryptString(cipher string) (string, error) {
	return strings.TrimPrefix(cipher, "enc:v1:"), nil
}

func strp(s string) *string { return &s }

// TestClustersCarryTheirEnvironment: an existing install gains the column, a cluster
// keeps the environment it was given, an empty value unlinks it, and an id that could
// not be one is refused before it is stored.
func TestClustersCarryTheirEnvironment(t *testing.T) {
	store := legacyStore(t)
	ctx := context.Background()
	for range 2 {
		if err := store.ensureSchema(ctx); err != nil {
			t.Fatalf("ensureSchema on an existing install: %v", err)
		}
	}

	cluster, err := store.create(ctx, ClusterInput{
		Name: strp("lab"), Kubeconfig: strp("apiVersion: v1"), EnvironmentID: strp("d2abc123env"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cluster.EnvironmentID != "d2abc123env" {
		t.Fatalf("created cluster environment = %q", cluster.EnvironmentID)
	}
	listed, err := store.list(ctx)
	if err != nil || len(listed) != 1 || listed[0].EnvironmentID != "d2abc123env" {
		t.Fatalf("list = %+v (%v), want the environment on the row", listed, err)
	}

	unlinked, err := store.update(ctx, cluster.ID, ClusterInput{EnvironmentID: strp("")})
	if err != nil || unlinked.EnvironmentID != "" {
		t.Fatalf("unlink = %+v (%v)", unlinked, err)
	}
	kept, err := store.update(ctx, cluster.ID, ClusterInput{Name: strp("lab-2")})
	if err != nil || kept.EnvironmentID != "" || kept.Name != "lab-2" {
		t.Fatalf("an update without environmentId changed it: %+v (%v)", kept, err)
	}

	if _, err := store.update(ctx, cluster.ID, ClusterInput{EnvironmentID: strp("../etc")}); !errors.Is(err, errBadEnvironmentID) {
		t.Fatalf("a malformed environment id: err = %v, want errBadEnvironmentID", err)
	}
}
