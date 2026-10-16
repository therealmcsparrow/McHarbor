// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
)

// enterDemo puts the process in demo mode for one test and takes it out again.
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

// countingTransport fails every request it is handed and counts them, so a test
// can prove the forge client was never used at all.
type countingTransport struct{ calls atomic.Int64 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("test transport: the network must not be reached")
}

// forgeFixture is a repository whose URL points at a local server that counts
// every hit, with a token, a webhook secret and one active preview whose PR the
// probe would ask about.
type forgeFixture struct {
	svc       *Service
	hits      *atomic.Int64
	transport *countingTransport
}

func newForgeFixture(t *testing.T) forgeFixture {
	t.Helper()
	database := newTestDB(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	mustExec(t, database,
		`INSERT INTO git_repos (id, name, url, branch, auth_type, credential_ref, webhook_secret, created_at, updated_at)
		 VALUES ('repo-f', 'shop', ?, 'main', 'token', 'forge-token-1234', 'whsec_test', ?, ?)`,
		srv.URL+"/acme/shop.git", stamp, stamp)
	mustExec(t, database,
		`INSERT INTO environments (id, name, connection_type, is_default, is_active, created_at, updated_at)
		 VALUES ('env-dev', 'dev', 'agent', 1, 1, ?, ?)`, stamp, stamp)
	mustExec(t, database,
		`INSERT INTO gitops_pipelines (id, name, repo_id, enabled, trigger_type, created_at, updated_at)
		 VALUES ('pipe-f', 'Shop', 'repo-f', 1, 'auto', ?, ?)`, stamp, stamp)
	mustExec(t, database,
		`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, target_environment_id, deploy_mode,
		        branch, compose_path, requires_approval, last_seen_commit, created_at, updated_at)
		 VALUES ('stage-f', 'pipe-f', 'dev', 0, 'env-dev', 'auto', 'main', '', 0, ?, ?, ?)`,
		"1111111111111111111111111111111111111111", stamp, stamp)
	mustExec(t, database,
		`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, source_branch, target_branch, commit_sha,
		        status, expires_at, opened_at, created_at, updated_at)
		 VALUES ('prev-f', 'pipe-f', 'repo-f', '7', 'feat/x', 'main', ?, 'active', ?, ?, ?, ?)`,
		"2222222222222222222222222222222222222222", now.Add(-time.Hour).Format(time.RFC3339),
		now.Add(-5*time.Hour).Format(time.RFC3339), stamp, stamp)

	svc := NewService(database, nil)
	transport := &countingTransport{}
	svc.forgeClient = &http.Client{Transport: transport}
	svc.SetDataDir(t.TempDir())
	t.Cleanup(svc.Close)
	return forgeFixture{svc: svc, hits: &hits, transport: transport}
}

func (f forgeFixture) assertSilent(t *testing.T) {
	t.Helper()
	if n := f.hits.Load(); n != 0 {
		t.Fatalf("the forge server was reached %d times in demo mode", n)
	}
	if n := f.transport.calls.Load(); n != 0 {
		t.Fatalf("the forge HTTP client was used %d times in demo mode", n)
	}
}

func TestDemoModeKeepsEveryForgePathOffline(t *testing.T) {
	enterDemo(t)
	f := newForgeFixture(t)
	ctx := context.Background()

	if _, err := f.svc.RemoteTip(ctx, "repo-f", "main"); !errors.Is(err, ErrForgeOffline) {
		t.Fatalf("RemoteTip in demo mode: got %v, want ErrForgeOffline", err)
	}
	if _, err := f.svc.fetchCompose(ctx, composeRequest{RepoID: "repo-f", Branch: "main",
		Commit: "1111111111111111111111111111111111111111", WorkID: "w1"}); !errors.Is(err, ErrForgeOffline) {
		t.Fatalf("fetchCompose in demo mode: got %v, want ErrForgeOffline", err)
	}
	if err := f.svc.postForgeComment(ctx, "repo-f", "7", "hello"); !errors.Is(err, ErrForgeOffline) {
		t.Fatalf("postForgeComment in demo mode: got %v, want ErrForgeOffline", err)
	}
	if err := f.svc.probePreviews(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("probePreviews: %v", err)
	}
	f.assertSilent(t)

	var status string
	if err := f.svc.db.QueryRow(`SELECT status FROM gitops_pr_previews WHERE id = 'prev-f'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != previewActive {
		t.Fatalf("the probe acted on a preview it could not ask about: status %s", status)
	}
}

func TestDemoModeSchedulerTickDoesNothing(t *testing.T) {
	enterDemo(t)
	f := newForgeFixture(t)
	sched := NewScheduler(f.svc, time.Minute)
	sched.tick(context.Background())
	f.assertSilent(t)

	var status string
	if err := f.svc.db.QueryRow(`SELECT status FROM gitops_pr_previews WHERE id = 'prev-f'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != previewActive {
		t.Fatalf("the reaper ran in demo mode and moved an expired seeded preview to %s", status)
	}
}

func TestForgeGateIsOpenOutsideDemoMode(t *testing.T) {
	f := newForgeFixture(t)
	if _, err := f.svc.RemoteTip(context.Background(), "repo-f", "main"); errors.Is(err, ErrForgeOffline) {
		t.Fatal("RemoteTip refused as offline on a normal boot")
	}
	if f.hits.Load() == 0 {
		t.Fatal("a normal boot did not reach the remote, so the demo test above proves nothing")
	}
}
