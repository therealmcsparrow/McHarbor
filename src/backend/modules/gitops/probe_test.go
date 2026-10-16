// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDecideProbeTable is the TOFIX-06 decision table: only a definite answer
// from an authenticated request destroys a preview.
func TestDecideProbeTable(t *testing.T) {
	cases := []struct {
		name       string
		prStatus   int
		prBody     string
		repoStatus int
		destroy    bool
		reason     string
	}{
		{"404 with the repository visible", 404, ``, 200, true, "pr-not-found"},
		{"404 with the repository also 404", 404, ``, 404, false, ""},
		{"404 with the repository 401", 404, ``, 401, false, ""},
		{"401", 401, ``, 0, false, ""},
		{"403", 403, ``, 0, false, ""},
		{"429", 429, ``, 0, false, ""},
		{"500", 500, ``, 0, false, ""},
		{"502", 502, ``, 0, false, ""},
		{"200 open (GitHub/Gitea)", 200, `{"state":"open","merged":false}`, 0, false, ""},
		{"200 opened (GitLab)", 200, `{"state":"opened"}`, 0, false, ""},
		{"200 locked (GitLab)", 200, `{"state":"locked"}`, 0, false, ""},
		{"200 closed not merged", 200, `{"state":"closed","merged":false}`, 0, true, "pr-closed"},
		{"200 closed and merged (GitHub/Gitea)", 200, `{"state":"closed","merged":true}`, 0, true, "pr-merged"},
		{"200 merged (GitLab)", 200, `{"state":"merged"}`, 0, true, "pr-merged"},
		{"200 closed (GitLab)", 200, `{"state":"closed"}`, 0, true, "pr-closed"},
		{"200 unknown state", 200, `{"state":"weird"}`, 0, false, ""},
		{"200 not JSON", 200, `<html>`, 0, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := decideProbe(tc.prStatus, []byte(tc.prBody), tc.repoStatus)
			if v.destroy != tc.destroy || v.reason != tc.reason {
				t.Fatalf("verdict = %+v, want destroy=%v reason=%q", v, tc.destroy, tc.reason)
			}
		})
	}
}

// fakeForge answers every forge's pull-request and repository endpoints from one
// httptest server, and refuses any request that does not carry the credential.
type fakeForge struct {
	t          *testing.T
	server     *httptest.Server
	prStatus   atomic.Int64
	prBody     atomic.Value
	repoStatus atomic.Int64
	hang       atomic.Bool
	prCalls    atomic.Int64
	mu         sync.Mutex
	paths      []string
}

func newFakeForge(t *testing.T) *fakeForge {
	t.Helper()
	f := &fakeForge{t: t}
	f.prStatus.Store(200)
	f.prBody.Store(`{"state":"open"}`)
	f.repoStatus.Store(200)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		auth := r.Header.Get("Authorization") + r.Header.Get("PRIVATE-TOKEN")
		if !strings.Contains(auth, "s3cret-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		isPR := strings.Contains(r.URL.Path, "/pulls/") || strings.Contains(r.URL.Path, "/merge_requests/")
		if isPR {
			f.prCalls.Add(1)
			if f.hang.Load() {
				<-r.Context().Done()
				return
			}
			w.WriteHeader(int(f.prStatus.Load()))
			if _, err := w.Write([]byte(f.prBody.Load().(string))); err != nil {
				t.Errorf("writing the fake PR body: %v", err)
			}
			return
		}
		w.WriteHeader(int(f.repoStatus.Load()))
	}))
	t.Cleanup(f.server.Close)
	return f
}

// redirectTransport sends every request to the fake forge while keeping the path
// the service built, so GitHub's fixed api.github.com host is exercised too.
type redirectTransport struct{ target *url.URL }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	clone.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func (f *fakeForge) client(timeout time.Duration) *http.Client {
	target, err := url.Parse(f.server.URL)
	if err != nil {
		f.t.Fatalf("parsing the fake forge URL: %v", err)
	}
	return &http.Client{Timeout: timeout, Transport: redirectTransport{target: target}}
}

func (f *fakeForge) seenPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

// seedProbedPreview creates a repository with the given URL and credential, and
// one active preview for pull request 7 with nothing deployed.
func seedProbedPreview(t *testing.T, database *sql.DB, repoURL, authType, credential string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database,
		`INSERT INTO git_repos (id, name, url, branch, auth_type, credential_ref, created_at, updated_at)
		 VALUES ('repo-p', 'repo-p', ?, 'main', ?, ?, ?, ?)`, repoURL, authType, credential, now, now)
	mustExec(t, database,
		`INSERT INTO gitops_pipelines (id, name, repo_id, enabled, trigger_type, created_at, updated_at)
		 VALUES ('pipe-p', 'pipe-p', 'repo-p', 1, 'pr_preview', ?, ?)`, now, now)
	mustExec(t, database,
		`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, source_branch, target_branch, commit_sha, status, expires_at, opened_at, created_at, updated_at)
		 VALUES ('prev-7', 'pipe-p', 'repo-p', '7', 'feature/x', 'main', 'sha', 'active', ?, ?, ?, ?)`,
		time.Now().UTC().Add(48*time.Hour).Format(time.RFC3339), now, now, now)
}

func newProbeService(t *testing.T, database *sql.DB, forge *fakeForge, timeout time.Duration) *Service {
	t.Helper()
	svc, _ := newPreviewService(t, database, allowAll{})
	svc.forgeClient = forge.client(timeout)
	return svc
}

// TestProbeActsOnlyOnDefiniteAnswers drives the reaper tick against each forge
// and each outcome, and checks what happened to the preview row.
func TestProbeActsOnlyOnDefiniteAnswers(t *testing.T) {
	forges := map[string]string{
		"github": "https://github.com/acme/app.git",
		"gitlab": "https://gitlab.example.com/acme/app.git",
		"gitea":  "https://git.example.com/acme/app.git",
	}
	outcomes := []struct {
		name       string
		prStatus   int
		prBody     string
		repoStatus int
		hang       bool
		wantStatus string
		wantBy     string
	}{
		{"authenticated 404", 404, ``, 200, false, previewDemolished, "pr-not-found"},
		{"404 on a repository the token cannot see", 404, ``, 404, false, previewActive, ""},
		{"401", 401, ``, 200, false, previewActive, ""},
		{"403", 403, ``, 200, false, previewActive, ""},
		{"500", 500, ``, 200, false, previewActive, ""},
		{"timeout", 200, `{"state":"closed"}`, 200, true, previewActive, ""},
		{"still open", 200, `{"state":"open"}`, 200, false, previewActive, ""},
		{"closed", 200, `{"state":"closed","merged":false}`, 200, false, previewDemolished, "pr-closed"},
		{"merged", 200, `{"state":"closed","merged":true}`, 200, false, previewDemolished, "pr-merged"},
	}
	for forgeName, repoURL := range forges {
		for _, oc := range outcomes {
			t.Run(forgeName+"/"+oc.name, func(t *testing.T) {
				if forgeName == "gitlab" && oc.name == "merged" {
					oc.prBody = `{"state":"merged"}`
				}
				database := newTestDB(t)
				forge := newFakeForge(t)
				forge.prStatus.Store(int64(oc.prStatus))
				forge.prBody.Store(oc.prBody)
				forge.repoStatus.Store(int64(oc.repoStatus))
				forge.hang.Store(oc.hang)
				seedProbedPreview(t, database, repoURL, "token", "s3cret-token")
				svc := newProbeService(t, database, forge, 200*time.Millisecond)

				if err := svc.ReapPreviews(context.Background()); err != nil {
					t.Fatalf("reap: %v", err)
				}
				status, _, tornDownBy := previewStatus(t, database, "prev-7")
				if status != oc.wantStatus || tornDownBy != oc.wantBy {
					t.Fatalf("status=%q torn_down_by=%q, want %q/%q (paths %v)", status, tornDownBy, oc.wantStatus, oc.wantBy, forge.seenPaths())
				}
				if forge.prCalls.Load() != 1 {
					t.Fatalf("pull request calls = %d, want 1", forge.prCalls.Load())
				}
			})
		}
	}
}

// TestProbeUsesEachForgesEndpoint pins the three URL shapes, so a probe never
// asks GitLab for a GitHub path and reads the 404 as "gone".
func TestProbeUsesEachForgesEndpoint(t *testing.T) {
	cases := map[string][2]string{
		"https://github.com/acme/app.git":         {"/repos/acme/app/pulls/7", "/repos/acme/app"},
		"https://gitlab.example.com/acme/app.git": {"/api/v4/projects/acme%2Fapp/merge_requests/7", "/api/v4/projects/acme%2Fapp"},
		"https://git.example.com/acme/app.git":    {"/api/v1/repos/acme/app/pulls/7", "/api/v1/repos/acme/app"},
	}
	for repoURL, want := range cases {
		api, err := resolveForge(repoURL)
		if err != nil {
			t.Fatalf("%s: %v", repoURL, err)
		}
		pr, err := url.Parse(api.pullEndpoint("7"))
		if err != nil {
			t.Fatalf("%s: %v", repoURL, err)
		}
		repo, err := url.Parse(api.repoEndpoint())
		if err != nil {
			t.Fatalf("%s: %v", repoURL, err)
		}
		if pr.EscapedPath() != want[0] || repo.EscapedPath() != want[1] {
			t.Fatalf("%s: pull=%s repo=%s, want %v", repoURL, pr.EscapedPath(), repo.EscapedPath(), want)
		}
	}
}

// TestProbeWithoutACredentialMakesNoRequest: an unauthenticated 404 proves
// nothing, so a repository with no credential is never asked at all.
func TestProbeWithoutACredentialMakesNoRequest(t *testing.T) {
	database := newTestDB(t)
	forge := newFakeForge(t)
	forge.prStatus.Store(404)
	seedProbedPreview(t, database, "https://github.com/acme/app.git", "none", "")
	svc := newProbeService(t, database, forge, time.Second)

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if status, _, _ := previewStatus(t, database, "prev-7"); status != previewActive {
		t.Fatalf("status = %q, want active", status)
	}
	if paths := forge.seenPaths(); len(paths) != 0 {
		t.Fatalf("requests made without a credential: %v", paths)
	}
}

// TestProbeIsRateLimited: at most probeBatch previews per tick, and a preview
// probed this interval is not asked again on the next tick.
func TestProbeIsRateLimited(t *testing.T) {
	database := newTestDB(t)
	forge := newFakeForge(t)
	seedProbedPreview(t, database, "https://github.com/acme/app.git", "token", "s3cret-token")
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 8; i < 8+probeBatch+2; i++ {
		mustExec(t, database,
			`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, source_branch, target_branch, commit_sha, status, opened_at, created_at, updated_at)
			 VALUES (?, 'pipe-p', 'repo-p', ?, 'b', 'main', 'sha', 'active', ?, ?, ?)`,
			fmt.Sprintf("prev-%d", i), fmt.Sprintf("%d", i), now, now, now)
	}
	svc := newProbeService(t, database, forge, time.Second)

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if got := forge.prCalls.Load(); got != probeBatch {
		t.Fatalf("first tick asked about %d pull requests, want %d", got, probeBatch)
	}
	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if got := forge.prCalls.Load(); got != probeBatch+3 {
		t.Fatalf("second tick total = %d, want %d (only the unprobed remainder)", got, probeBatch+3)
	}
}
