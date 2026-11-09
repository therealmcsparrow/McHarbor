// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// useTempRoot points the env override at a fresh directory, so acquireRunTemp resolves it at call time.
func useTempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(tempDirEnv, root)
	return root
}

func mustAcquire(t *testing.T, ctx context.Context, runID string) (context.Context, string, func()) {
	t.Helper()
	ctx2, dir, release, err := acquireRunTemp(ctx, runID)
	if err != nil {
		t.Fatalf("acquireRunTemp(%q): %v", runID, err)
	}
	if release == nil {
		t.Fatalf("acquireRunTemp(%q) returned a nil release", runID)
	}
	t.Cleanup(release)
	return ctx2, dir, release
}

func writeInside(t *testing.T, dir string) {
	t.Helper()
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "spool.bin"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write spool: %v", err)
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, stat err = %v", path, err)
	}
}

func TestAcquireRunTempCreatesNamedFolderOnCurrentRoot(t *testing.T) {
	root := useTempRoot(t)
	ctx2, dir, _ := mustAcquire(t, context.Background(), "run-create")

	if want := filepath.Join(root, "mcharbor-run-run-create"); dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("run folder missing: %v", err)
	}
	if runtime.GOOS != "windows" {
		fi, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("stat run folder: %v", statErr)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Fatalf("run folder mode = %o, want 0700", perm)
		}
	}
	if got := runTempDir(ctx2); got != dir {
		t.Fatalf("runTempDir(ctx2) = %q, want %q", got, dir)
	}
	if !tempPathOwned(dir) {
		t.Fatal("an active run folder must read as owned")
	}
}

func TestRunTempDirFallsBackToCurrentRoot(t *testing.T) {
	root := useTempRoot(t)
	if got := runTempDir(context.Background()); got != root {
		t.Fatalf("runTempDir(plain ctx) = %q, want current root %q", got, root)
	}
}

func TestRunTempRemovedOnEveryExit(t *testing.T) {
	boom := errors.New("backup failed")
	cases := []struct {
		name string
		run  func(ctx context.Context, cancel context.CancelFunc) error
	}{
		{"success", func(context.Context, context.CancelFunc) error { return nil }},
		{"error", func(context.Context, context.CancelFunc) error { return boom }},
		{"cancelled", func(ctx context.Context, cancel context.CancelFunc) error {
			cancel()
			return ctx.Err()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTempRoot(t)
			before := activeTempOwners()
			base, cancel := context.WithCancel(context.Background())
			defer cancel()

			var dir string
			err := func() error {
				ctx2, d, release, aerr := acquireRunTemp(base, "run-exit-"+tc.name)
				if aerr != nil {
					return aerr
				}
				defer release()
				dir = d
				writeInside(t, runTempDir(ctx2))
				return tc.run(ctx2, cancel)
			}()
			if tc.name == "success" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.name != "success" && err == nil {
				t.Fatal("expected the run body to report its error")
			}
			if dir == "" {
				t.Fatal("run folder was never reported")
			}
			assertGone(t, dir)
			if tempPathOwned(dir) {
				t.Fatal("a released run folder must not read as owned")
			}
			if got := activeTempOwners(); got != before {
				t.Fatalf("activeTempOwners = %d, want %d after release", got, before)
			}
		})
	}
}

func TestReleaseRunTempTwiceIsSafe(t *testing.T) {
	useTempRoot(t)
	before := activeTempOwners()
	_, dir, release := mustAcquire(t, context.Background(), "run-double")
	writeInside(t, dir)

	release()
	release()

	assertGone(t, dir)
	if got := activeTempOwners(); got != before {
		t.Fatalf("activeTempOwners = %d, want %d: a double release must not drive the count off", got, before)
	}
}

func TestSecondReleaseDoesNotRemoveReusedRunID(t *testing.T) {
	useTempRoot(t)
	_, _, release := mustAcquire(t, context.Background(), "run-reuse")
	release()

	_, dir2, release2 := mustAcquire(t, context.Background(), "run-reuse")
	writeInside(t, dir2)
	release()

	if _, err := os.Stat(dir2); err != nil {
		t.Fatalf("a stale release removed a newer run's folder: %v", err)
	}
	release2()
	assertGone(t, dir2)
}

func TestRunStaysOnItsStartRootAfterRootChange(t *testing.T) {
	oldRoot := useTempRoot(t)
	ctx2, dir, release := mustAcquire(t, context.Background(), "run-moved")
	writeInside(t, dir)

	newRoot := t.TempDir()
	t.Setenv(tempDirEnv, newRoot)

	if got := runTempDir(ctx2); got != dir || !strings.HasPrefix(got, oldRoot) {
		t.Fatalf("runTempDir(ctx2) = %q, want it to stay under old root %q", got, oldRoot)
	}
	if got := runTempDir(context.Background()); got != newRoot {
		t.Fatalf("runTempDir(plain ctx) = %q, want new root %q", got, newRoot)
	}
	_, dirNew, releaseNew := mustAcquire(t, context.Background(), "run-after-move")
	if !strings.HasPrefix(dirNew, newRoot) {
		t.Fatalf("a run started after the change used %q, want under %q", dirNew, newRoot)
	}

	release()
	assertGone(t, dir)
	if _, err := os.Stat(newRoot); err != nil {
		t.Fatalf("releasing the old run must not touch the new root: %v", err)
	}
	if _, err := os.Stat(dirNew); err != nil {
		t.Fatalf("releasing the old run removed the new run's folder: %v", err)
	}
	releaseNew()
	assertGone(t, dirNew)
}

func TestOwnedPartialReadsOwnedUntilReleased(t *testing.T) {
	dest := t.TempDir()
	partial := filepath.Join(dest, ".mcharbor-backup-abc123")
	if err := os.WriteFile(partial, []byte("half"), 0o600); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	before := activeTempOwners()

	if tempPathOwned(partial) {
		t.Fatal("an unregistered partial must not read as owned")
	}
	release := ownTempPath(partial)
	if release == nil {
		t.Fatal("ownTempPath returned a nil release")
	}
	t.Cleanup(release)

	if !tempPathOwned(partial) {
		t.Fatal("a registered partial must read as owned")
	}
	if !tempPathOwned(filepath.Join(partial, "child")) {
		t.Fatal("a path under a registered path must read as owned")
	}
	if tempPathOwned(filepath.Join(dest, ".mcharbor-backup-other")) {
		t.Fatal("a sibling must not read as owned")
	}
	if got := activeTempOwners(); got != before+1 {
		t.Fatalf("activeTempOwners = %d, want %d", got, before+1)
	}

	release()
	release()

	if tempPathOwned(partial) {
		t.Fatal("a released partial must not read as owned")
	}
	if got := activeTempOwners(); got != before {
		t.Fatalf("activeTempOwners = %d, want %d after release", got, before)
	}
}

func TestOwnedPrefixIsNotMatchedByNameSibling(t *testing.T) {
	dest := t.TempDir()
	owned := filepath.Join(dest, ".mcharbor-agent-backup-1")
	release := ownTempPath(owned)
	t.Cleanup(release)

	if tempPathOwned(owned + "2") {
		t.Fatal("a path that only shares a name prefix must not read as owned")
	}
}

func TestAcquireRunTempRefusesExistingFolderAndBadIDs(t *testing.T) {
	root := useTempRoot(t)
	_, dir, _ := mustAcquire(t, context.Background(), "run-dup")
	writeInside(t, dir)

	if _, _, _, err := acquireRunTemp(context.Background(), "run-dup"); err == nil {
		t.Fatal("acquiring an existing run folder must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "spool.bin")); err != nil {
		t.Fatalf("a refused acquire must leave the live folder intact: %v", err)
	}
	for _, bad := range []string{"", "../escape", "a/b", `a\b`, "has space", strings.Repeat("x", 65)} {
		if _, _, _, err := acquireRunTemp(context.Background(), bad); err == nil {
			t.Fatalf("run id %q must be refused", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); err == nil {
		t.Fatal("a path-traversal run id created a folder outside the root")
	}
}
