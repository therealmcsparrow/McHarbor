// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
)

func enterDemo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	marker, err := coredemo.NewMarker("u")
	if err != nil {
		t.Fatal(err)
	}
	if err := coredemo.WriteMarker(dir, marker); err != nil {
		t.Fatal(err)
	}
	if _, err := coredemo.Load(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := coredemo.Load(t.TempDir()); err != nil {
			t.Error(err)
		}
	})
}

// syncFixture is a migrated database with two repositories whose URLs point at a
// local server that counts every request: one healthy, one whose last sync failed.
type syncFixture struct {
	svc  *Service
	hits *atomic.Int64
}

const deployedSHA = "9f3c2a1e5b7d4c6a8e0f1b2d3c4e5f6a7b8c9d0e"

func newSyncFixture(t *testing.T) syncFixture {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "git.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	stamp := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO git_repos (id, name, url, branch, auth_type, created_at, updated_at) VALUES ('ok', 'shop', ?, 'main', 'none', ?, ?)`,
			[]any{srv.URL + "/acme/shop.git", stamp, stamp}},
		{`INSERT INTO git_repos (id, name, url, branch, auth_type, last_sync_error, created_at, updated_at) VALUES ('bad', 'infra', ?, 'main', 'none', 'authentication required', ?, ?)`,
			[]any{srv.URL + "/acme/infra.git", stamp, stamp}},
		{`INSERT INTO git_deployments (id, repo_id, commit_sha, branch, status, message, created_at, updated_at) VALUES ('d1', 'ok', ?, 'main', 'success', 'Sync completed', ?, ?)`,
			[]any{deployedSHA, stamp, stamp}},
	} {
		if _, err := database.Exec(q.query, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return syncFixture{svc: NewService(database, nil), hits: &hits}
}

func (f syncFixture) repo(t *testing.T, id string) *Repo {
	t.Helper()
	r, err := f.svc.RepoByID(context.Background(), id)
	if err != nil || r == nil {
		t.Fatalf("reading repo %s: %v", id, err)
	}
	return r
}

func TestDemoSyncNeverReachesTheRemote(t *testing.T) {
	enterDemo(t)
	f := newSyncFixture(t)
	ctx := context.Background()

	res, err := f.svc.Sync(ctx, f.repo(t, "ok"), t.TempDir())
	if err != nil {
		t.Fatalf("demo sync of a healthy repository: %v", err)
	}
	if res.CommitSHA != deployedSHA {
		t.Fatalf("demo sync landed on %s, want the last deployed commit %s", res.CommitSHA, deployedSHA)
	}
	if _, err := f.svc.Sync(ctx, f.repo(t, "bad"), t.TempDir()); err == nil || err.Error() != "authentication required" {
		t.Fatalf("demo sync of a failing repository: got %v, want its recorded error", err)
	}
	if _, _, _, err := f.svc.RemoteAccess(ctx, "ok"); !errors.Is(err, ErrRemoteOffline) {
		t.Fatalf("RemoteAccess in demo mode: got %v, want ErrRemoteOffline", err)
	}
	if n := f.hits.Load(); n != 0 {
		t.Fatalf("the remote was reached %d times in demo mode", n)
	}
}

func TestSyncReachesTheRemoteOutsideDemoMode(t *testing.T) {
	f := newSyncFixture(t)
	if _, err := f.svc.Sync(context.Background(), f.repo(t, "ok"), t.TempDir()); err == nil {
		t.Fatal("a sync against a server that answers 404 succeeded")
	}
	if f.hits.Load() == 0 {
		t.Fatal("a normal boot did not reach the remote, so the demo test proves nothing")
	}
}
