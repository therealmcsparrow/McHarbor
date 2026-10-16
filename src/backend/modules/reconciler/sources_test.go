// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
)

type localRemote struct {
	url   string
	token string
}

func (l localRemote) RemoteAccess(context.Context, string) (string, transport.AuthMethod, string, error) {
	return l.url, nil, l.token, nil
}

// gitRepo is a repository on disk the tests commit to and sync from.
type gitRepo struct {
	t    *testing.T
	dir  string
	repo *gogit.Repository
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &gitRepo{t: t, dir: dir, repo: repo}
}

func (g *gitRepo) commit(files map[string]string, remove ...string) {
	g.t.Helper()
	wt, err := g.repo.Worktree()
	if err != nil {
		g.t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(g.dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			g.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			g.t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			g.t.Fatal(err)
		}
	}
	for _, name := range remove {
		if _, err := wt.Remove(name); err != nil {
			g.t.Fatal(err)
		}
	}
	if _, err := wt.Commit("test", &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@t", When: time.Now()}}); err != nil {
		g.t.Fatal(err)
	}
}

func newSyncer(t *testing.T, database *sql.DB, remote localRemote) (*SourceSyncer, *Source) {
	t.Helper()
	s := NewSourceSyncer(database, testEncryption(t), remote, coreAudit.NewLogger(database, "test-secret"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('env-git', 'git') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	src, err := s.Create(context.Background(), SourceInput{Name: "intent", RepoID: "r1", Branch: "main", Path: "desired", EnvID: "env-git"})
	if err != nil {
		t.Fatal(err)
	}
	return s, src
}

const webDoc = `# the public site
name: web
kind: container
containerName: web
imageRef: nginx:1.27
restartPolicy: unless-stopped
mode: propose
config:
  env:
    MODE: prod
`

// TestSyncMakesTheTableMatchTheRepository covers create, update, delete, the
// read-only rule for Git rows, and that an unchanged branch is not re-read.
func TestSyncMakesTheTableMatchTheRepository(t *testing.T) {
	_, database, _, _ := newLoopFixture(t)
	g := newGitRepo(t)
	g.commit(map[string]string{
		"desired/web.yaml": webDoc,
		"desired/more.yaml": "desiredStates:\n  - name: api\n    containerName: api\n    imageRef: api:3\n" +
			"  - name: shop\n    kind: stack\n    stackName: shop\n    composeFile: stacks/shop.yaml\n",
		"stacks/shop.yaml": "services:\n  web:\n    image: shop\n",
		"README.md":        "not a document",
	})
	s, src := newSyncer(t, database, localRemote{url: g.dir})
	ctx := context.Background()

	res, err := s.Sync(ctx, src, false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Created != 3 || res.RowCount != 3 {
		t.Fatalf("result = %+v", res)
	}
	var mode, cfg, compose, env string
	if err := database.QueryRow(`SELECT mode, config, env_id FROM desired_states WHERE name = 'web'`).Scan(&mode, &cfg, &env); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg, "enc:v1:") {
		t.Fatalf("a synced config was stored in plaintext: %s", cfg)
	}
	if cfg, err = s.service.open(cfg); err != nil {
		t.Fatal(err)
	}
	if mode != ModePropose || env != "env-git" || !strings.Contains(cfg, `"MODE":"prod"`) {
		t.Fatalf("web row: mode=%s env=%s config=%s", mode, env, cfg)
	}
	if err := database.QueryRow(`SELECT source_compose FROM desired_states WHERE name = 'shop'`).Scan(&compose); err != nil || !strings.Contains(compose, "image: shop") {
		t.Fatalf("stack compose from the repository was not stored: %q %v", compose, err)
	}

	src = mustGetSource(t, s, src.ID)
	if again, err := s.Sync(ctx, src, false); err != nil || again.Changed {
		t.Fatalf("an unchanged branch was re-read: %+v %v", again, err)
	}

	g.commit(map[string]string{"desired/web.yaml": strings.Replace(webDoc, "nginx:1.27", "nginx:1.28", 1)}, "desired/more.yaml")
	src = mustGetSource(t, s, src.ID)
	res, err = s.Sync(ctx, src, false)
	if err != nil || res.Updated != 1 || res.Deleted != 2 {
		t.Fatalf("second sync: %+v %v", res, err)
	}
	var image string
	if err := database.QueryRow(`SELECT image_ref FROM desired_states WHERE name = 'web'`).Scan(&image); err != nil || image != "nginx:1.28" {
		t.Fatalf("image after sync = %q %v", image, err)
	}
}

// TestSyncIsAllOrNothing checks one bad document fails the whole sync, leaves
// the rows alone, and records the reason on the source; enforce is refused.
func TestSyncIsAllOrNothing(t *testing.T) {
	_, database, _, _ := newLoopFixture(t)
	g := newGitRepo(t)
	g.commit(map[string]string{"desired/web.yaml": webDoc})
	s, src := newSyncer(t, database, localRemote{url: g.dir})
	ctx := context.Background()
	if _, err := s.Sync(ctx, src, false); err != nil {
		t.Fatal(err)
	}
	g.commit(map[string]string{
		"desired/web.yaml": strings.Replace(webDoc, "nginx:1.27", "nginx:9", 1),
		"desired/bad.yaml": "name: sneaky\ncontainerName: x\nimageRef: x:1\nmode: enforce\n",
	})
	src = mustGetSource(t, s, src.ID)
	if _, err := s.Sync(ctx, src, false); err == nil || !strings.Contains(err.Error(), "enforce") {
		t.Fatalf("a document asking for enforce was accepted: %v", err)
	}
	var image string
	if err := database.QueryRow(`SELECT image_ref FROM desired_states WHERE name = 'web'`).Scan(&image); err != nil || image != "nginx:1.27" {
		t.Fatalf("a failed sync changed a row: %q %v", image, err)
	}
	src = mustGetSource(t, s, src.ID)
	if !strings.Contains(src.LastError, "bad.yaml") {
		t.Fatalf("the failure was not recorded on the source: %q", src.LastError)
	}
}

// TestPullRequestWritesRealityBackToGit covers the drift-as-PR path end to end
// against a local repository: the document is rewritten with its comment kept,
// what cannot be expressed is listed, and the branch is pushed. A local path is
// no Git host, so opening the request is reported, not faked.
func TestPullRequestWritesRealityBackToGit(t *testing.T) {
	_, database, _, _ := newLoopFixture(t)
	g := newGitRepo(t)
	g.commit(map[string]string{"desired/web.yaml": webDoc})
	s, src := newSyncer(t, database, localRemote{url: g.dir, token: "tok"})
	ctx := context.Background()
	if _, err := s.Sync(ctx, src, false); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := database.QueryRow(`SELECT id FROM desired_states WHERE name = 'web'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	report, err := json.Marshal(DriftReport{HasDrift: true, Diffs: []DriftDiff{
		{Class: ClassConfiguration, Field: "image", Expected: "nginx:1.27", Actual: "nginx:1.29"},
		{Class: ClassConfiguration, Field: "env.MODE", Expected: "prod", Actual: "debug"},
		{Class: ClassConfiguration, Field: "mount./data", Expected: "data", Actual: "none"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE desired_states SET last_report = ?, drift_detected = 1 WHERE id = ?`, string(report), id); err != nil {
		t.Fatal(err)
	}
	ds, err := s.service.ByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.OpenPullRequest(ctx, ds, http.DefaultClient)
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	if len(res.Changed) != 2 || len(res.Skipped) != 1 || res.Skipped[0] != "mount./data" {
		t.Fatalf("changed=%v skipped=%v", res.Changed, res.Skipped)
	}
	if res.PullRequestURL != "" || !strings.Contains(res.ProviderError, "not an HTTP URL") {
		t.Fatalf("a local remote must report that no request was opened: %+v", res)
	}
	ref, err := g.repo.Reference(plumbing.NewBranchReferenceName(res.Branch), true)
	if err != nil {
		t.Fatalf("branch %s was not pushed: %v", res.Branch, err)
	}
	commit, err := g.repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	file, err := commit.File("desired/web.yaml")
	if err != nil {
		t.Fatal(err)
	}
	body, err := file.Contents()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# the public site", "imageRef: nginx:1.29", "MODE: debug"} {
		if !strings.Contains(body, want) {
			t.Errorf("rewritten document lacks %q:\n%s", want, body)
		}
	}
	main, err := g.repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	if main.Hash() == ref.Hash() {
		t.Fatal("the change landed on main instead of a branch")
	}
}

// TestCreatePullRequestPicksTheProviderAPI checks the Gitea request shape and
// that a refusal carries the host's own message.
func TestCreatePullRequestPicksTheProviderAPI(t *testing.T) {
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/repos/team/intent/pulls" {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding the pull request body: %v", err)
			}
			if _, err := w.Write([]byte(`{"html_url":"http://x/pr/1"}`)); err != nil {
				t.Errorf("writing the pull request reply: %v", err)
			}
			return
		}
		http.Error(w, "branch already has a pull request", http.StatusConflict)
	}))
	t.Cleanup(srv.Close)
	u, provider, err := createPullRequest(context.Background(), srv.Client(), srv.URL+"/team/intent.git", "tok", "b", "main", "T", "B")
	if err != nil || u != "http://x/pr/1" || provider != "gitea" || body["head"] != "b" || body["base"] != "main" {
		t.Fatalf("gitea: %q %q %v %v", u, provider, err, body)
	}
	_, _, err = createPullRequest(context.Background(), srv.Client(), srv.URL+"/other/x.git", "tok", "b", "main", "T", "B")
	if err == nil || !strings.Contains(err.Error(), "already has a pull request") {
		t.Fatalf("refusal lost the host's message: %v", err)
	}
}

func mustGetSource(t *testing.T, s *SourceSyncer, id string) *Source {
	t.Helper()
	src, err := s.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return src
}
