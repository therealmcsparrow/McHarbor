// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestStageLayoutKeepsEveryPathUnderBase(t *testing.T) {
	base := filepath.Join(string(filepath.Separator)+"run", "mcharbor-agent")
	layout, err := stageLayoutFor(base, "ct123")
	if err != nil {
		t.Fatalf("stageLayoutFor: %v", err)
	}
	wantOwned := []string{base, filepath.Join(base, "ct"), filepath.Join(base, "ct", "ct123")}
	if len(layout.Owned) != len(wantOwned) {
		t.Fatalf("owned = %v, want %v", layout.Owned, wantOwned)
	}
	for i := range wantOwned {
		if layout.Owned[i] != wantOwned[i] {
			t.Fatalf("owned = %v, want %v", layout.Owned, wantOwned)
		}
	}
	if layout.Dir != wantOwned[2] || layout.Target != filepath.Join(wantOwned[2], "rootfs") {
		t.Fatalf("dir = %q, target = %q", layout.Dir, layout.Target)
	}

	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "../x"} {
		if _, err := stageLayoutFor(base, bad); err == nil {
			t.Fatalf("id %q was accepted; a release could rmdir outside the staging root", bad)
		}
	}
}

func TestOnlyUnprivilegedContainersAreStaged(t *testing.T) {
	if !needsStaging(Spec{Privileged: false}) {
		t.Fatal("an unprivileged container was not staged; its mapped root cannot reach a rootfs under a 0700 pool")
	}
	if needsStaging(Spec{Privileged: true}) {
		t.Fatal("a privileged container was staged; host root needs no traversable path")
	}
}

func TestUntraversableAncestorNamesTheFirstClosedDirectory(t *testing.T) {
	sep := string(filepath.Separator)
	base := filepath.Join(sep+"run", "mcharbor-agent")
	modes := map[string]fs.FileMode{sep: 0o755, filepath.Dir(base): 0o755}
	modeOf := func(dir string) (fs.FileMode, error) {
		mode, ok := modes[dir]
		if !ok {
			return 0, fs.ErrNotExist
		}
		return mode, nil
	}

	if dir, err := untraversableAncestor(base, modeOf); err != nil || dir != "" {
		t.Fatalf("open chain = %q, %v; want none", dir, err)
	}

	modes[filepath.Dir(base)] = 0o700
	if dir, err := untraversableAncestor(base, modeOf); err != nil || dir != filepath.Dir(base) {
		t.Fatalf("closed /run = %q, %v; want %q", dir, err, filepath.Dir(base))
	}

	delete(modes, filepath.Dir(base))
	if _, err := untraversableAncestor(base, modeOf); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unreadable ancestor = %v, want the stat error", err)
	}
}

func TestStartupSweepKeepsTheStagingOfAnythingThatMayRun(t *testing.T) {
	cases := []struct {
		p     presence
		state string
		keep  bool
	}{
		{presenceAbsent, "", false},
		{presenceUnstarted, "", false},
		{presenceLoaded, StateStopped, false},
		{presenceLoaded, StateRunning, true},
		{presenceLoaded, StateFrozen, true},
		{presenceLoaded, StateDefined, true},
		{presenceLoaded, StateUnknown, true},
	}
	for _, c := range cases {
		if got := keepStageOnSweep(c.p, c.state); got != c.keep {
			t.Errorf("presence %d state %q: keep = %v, want %v", c.p, c.state, got, c.keep)
		}
	}
}
