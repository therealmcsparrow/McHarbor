// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// localRepo builds a two-commit repository on disk: the first commit's compose
// file says v1, the second's says v2, so a test can tell "the exact commit" from
// "the tip". It returns the path and both commit hashes.
func localRepo(t *testing.T, extra map[string][]byte) (string, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("the git binary is needed for go-git's local file transport")
	}
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit := func(files map[string][]byte, msg string) string {
		for name, content := range files {
			full := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, content, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := wt.Add(name); err != nil {
				t.Fatal(err)
			}
		}
		hash, err := wt.Commit(msg, &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}})
		if err != nil {
			t.Fatal(err)
		}
		return hash.String()
	}
	first := map[string][]byte{"deploy/compose.yml": []byte("version: v1\n")}
	for k, v := range extra {
		first[k] = v
	}
	c1 := commit(first, "first")
	c2 := commit(map[string][]byte{"deploy/compose.yml": []byte("version: v2\n")}, "second")
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Name().Short() != "master" && head.Name().Short() != "main" {
		t.Fatalf("unexpected default branch %s", head.Name())
	}
	return dir, c1, c2
}

func branchOf(t *testing.T, dir string) string {
	t.Helper()
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	return head.Name().Short()
}

func testLimits() checkoutLimits {
	l := defaultCheckoutLimits()
	l.timeout = time.Minute
	return l
}

// TestFetchReadsTheExactCommitNotTheTip: a promotion of an older commit deploys
// that commit's compose file, even though the branch has moved on.
func TestFetchReadsTheExactCommitNotTheTip(t *testing.T) {
	src, c1, c2 := localRepo(t, nil)
	branch := branchOf(t, src)

	got, err := fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branch, c1, "deploy/compose.yml", testLimits())
	if err != nil {
		t.Fatalf("fetch c1: %v", err)
	}
	if got != "version: v1\n" {
		t.Fatalf("c1 content = %q, want v1", got)
	}
	got, err = fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branch, c2, "deploy/compose.yml", testLimits())
	if err != nil || got != "version: v2\n" {
		t.Fatalf("c2 content = %q err=%v", got, err)
	}
}

// TestFetchRefusesOverTheSizeLimit: a repository larger than the cap is refused
// with a message naming the limit, not fetched to the end.
func TestFetchRefusesOverTheSizeLimit(t *testing.T) {
	blob := make([]byte, 512<<10)
	if _, err := rand.Read(blob); err != nil {
		t.Fatal(err)
	}
	src, c1, _ := localRepo(t, map[string][]byte{"big.bin": blob})
	limits := testLimits()
	limits.maxBytes = 64 << 10

	_, err := fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branchOf(t, src), c1, "deploy/compose.yml", limits)
	if err == nil || !strings.Contains(err.Error(), "checkout limit") {
		t.Fatalf("err = %v, want the size limit", err)
	}
}

// TestFetchRefusesOverTheTimeLimit: a fetch that cannot finish in time is
// refused with a message naming the bound.
func TestFetchRefusesOverTheTimeLimit(t *testing.T) {
	src, c1, _ := localRepo(t, nil)
	limits := testLimits()
	limits.timeout = time.Nanosecond

	_, err := fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branchOf(t, src), c1, "deploy/compose.yml", limits)
	if err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("err = %v, want the time limit", err)
	}
}

// TestFetchRefusesAnOversizedComposeFile: the compose file has its own cap.
func TestFetchRefusesAnOversizedComposeFile(t *testing.T) {
	src, c1, _ := localRepo(t, nil)
	limits := testLimits()
	limits.maxComposeBytes = 4

	_, err := fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branchOf(t, src), c1, "deploy/compose.yml", limits)
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("err = %v, want the compose size limit", err)
	}
}

// TestFetchNamesAMissingComposeFile: the operator reads which path was missing
// at which commit.
func TestFetchNamesAMissingComposeFile(t *testing.T) {
	src, c1, _ := localRepo(t, nil)

	_, err := fetchComposeAt(context.Background(), filepath.Join(t.TempDir(), "co"), src, nil, branchOf(t, src), c1, "docker-compose.yml", testLimits())
	if err == nil || !strings.Contains(err.Error(), "docker-compose.yml was not found at commit") {
		t.Fatalf("err = %v", err)
	}
}

// TestComposePathMustStayInsideTheRepository guards the one input an operator
// types that becomes a path.
func TestComposePathMustStayInsideTheRepository(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "/etc/passwd", "a/../../b", `..\x`, "."} {
		if _, err := cleanComposePath(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	for in, want := range map[string]string{"": defaultComposePath, "deploy/compose.yml": "deploy/compose.yml", `deploy\compose.yml`: "deploy/compose.yml", "./a/b.yml": "a/b.yml"} {
		got, err := cleanComposePath(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q, %v; want %q", in, got, err, want)
		}
	}
}

// TestServiceFetchLeavesNoCheckoutBehind: the per-promotion directory is gone
// the moment the compose file has been read.
func TestServiceFetchLeavesNoCheckoutBehind(t *testing.T) {
	src, c1, _ := localRepo(t, nil)
	database := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database,
		`INSERT INTO git_repos (id, name, url, branch, auth_type, created_at, updated_at) VALUES ('local', 'local', ?, 'main', 'none', ?, ?)`,
		src, now, now)
	svc := NewService(database, nil)
	t.Cleanup(svc.Close)
	dataDir := t.TempDir()
	svc.SetDataDir(dataDir)

	got, err := svc.fetchCompose(context.Background(), composeRequest{
		RepoID: "local", Branch: branchOf(t, src), Commit: c1, ComposePath: "deploy/compose.yml", WorkID: "prom-1",
	})
	if err != nil || got != "version: v1\n" {
		t.Fatalf("got %q err=%v", got, err)
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "gitops", "checkouts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("checkouts left behind: %d", len(entries))
	}
}
