// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"os"
	"testing"
	"time"
)

// publishBuild puts a rebuilt release of version in the mirror's per-build
// folder and points latest.json at it, the way the catalog publishes a new BUILD.
func (rs *releaseServer) publishBuild(t *testing.T, prefix, version, build string, files releaseFiles) {
	t.Helper()
	for name, body := range files {
		rs.put(prefix+"/"+version+"/"+build+"/"+name, body)
	}
	rs.put(prefix+"/latest.json", []byte(`{"version":"`+version+`","build":"`+build+`"}`))
}

const (
	firstBuild   = "202609281200"
	rebuiltBuild = "202610010900"
)

// storeWithRebuild stores firstBuild of 2026.09.02 as the target, then publishes
// rebuiltBuild of the same version, the way the catalog ships a new BUILD.
func storeWithRebuild(t *testing.T) (*Store, *releaseServer) {
	t.Helper()
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	rs.publishMirror("/m", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
	ctx := t.Context()
	if _, err := s.Pull(ctx, "2026.09.02", "", "tester"); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if err := s.SetTarget(ctx, "2026.09.02", ""); err != nil {
		t.Fatal(err)
	}
	rebuilt := releaseFiles{
		"mcharbor-agent-linux-amd64":       []byte("linux-amd64 agent 2026.09.02 rebuilt"),
		"mcharbor-agent-windows-amd64.exe": []byte("windows agent 2026.09.02 rebuilt"),
		"BUILD":                            []byte(rebuiltBuild + "\n"),
	}
	rebuilt.resign(priv)
	rs.publishBuild(t, "/m", "2026.09.02", rebuiltBuild, rebuilt)
	return s, rs
}

func servedBinary(t *testing.T, s *Store) (build, body string) {
	t.Helper()
	path, _, build := s.TargetBinaryPath(t.Context(), "linux", "amd64")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading served binary %q: %v", path, err)
	}
	return build, string(got)
}

// TestANewerBuildLandsBesideTheStoredOne: a rebuild of the stored version is
// stored as its own release, and the build agents ran before stays selectable.
func TestANewerBuildLandsBesideTheStoredOne(t *testing.T) {
	s, _ := storeWithRebuild(t)
	ctx := t.Context()

	if rel, err := s.Pull(ctx, "2026.09.02", "", "tester"); err != nil || rel.Build != firstBuild {
		t.Fatalf("before a check sees the new build, the stored one stands: %+v, %v", rel, err)
	}
	if _, err := s.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}
	rel, err := s.Pull(ctx, "2026.09.02", "", "tester")
	if err != nil || rel.Build != rebuiltBuild || rel.Status != StatusReady {
		t.Fatalf("pull of the newer build: %+v, %v", rel, err)
	}

	old, err := s.Get(ctx, "2026.09.02", firstBuild)
	if err != nil || old.Status != StatusReady {
		t.Fatalf("the previous build must still be stored and ready: %+v, %v", old, err)
	}
	if build, body := servedBinary(t, s); build != firstBuild || body != "linux-amd64 agent 2026.09.02" {
		t.Fatalf("a pull alone must not move the target: build %q, body %q", build, body)
	}

	if err := s.SetTarget(ctx, "2026.09.02", rebuiltBuild); err != nil {
		t.Fatal(err)
	}
	if build, body := servedBinary(t, s); build != rebuiltBuild || body != "linux-amd64 agent 2026.09.02 rebuilt" {
		t.Fatalf("target on the new build: build %q, body %q", build, body)
	}
	if err := s.SetTarget(ctx, "2026.09.02", firstBuild); err != nil {
		t.Fatalf("stepping back to the previous build: %v", err)
	}
	if build, body := servedBinary(t, s); build != firstBuild || body != "linux-amd64 agent 2026.09.02" {
		t.Fatalf("target back on the old build: build %q, body %q", build, body)
	}
	if err := s.Delete(ctx, "2026.09.02", ""); err == nil {
		t.Fatal("a delete that names no build must not guess between two")
	}
	if err := s.Delete(ctx, "2026.09.02", rebuiltBuild); err != nil {
		t.Fatalf("deleting the build that is not the target: %v", err)
	}
	if build, _ := servedBinary(t, s); build != firstBuild {
		t.Fatalf("deleting another build disturbed the target: %q", build)
	}
}

// TestDownloadSetsTheTargetAndKeepsTheOldOne: the operator's download is the
// StartPull with andTarget; it moves the target to the new build and the old
// one stays stored to switch back to.
func TestDownloadSetsTheTargetAndKeepsTheOldOne(t *testing.T) {
	s, _ := storeWithRebuild(t)
	ctx := t.Context()
	if _, err := s.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}

	if err := s.StartPull(ctx, "2026.09.02", rebuiltBuild, "tester", true); err != nil {
		t.Fatalf("start pull: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if rel, ok := s.TargetRelease(ctx); ok && rel.Build == rebuiltBuild {
			break
		}
		if time.Now().After(deadline) {
			rel, err := s.Get(ctx, "2026.09.02", rebuiltBuild)
			t.Fatalf("the download never became the target: %+v, %v", rel, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if build, body := servedBinary(t, s); build != rebuiltBuild || body != "linux-amd64 agent 2026.09.02 rebuilt" {
		t.Fatalf("served after download: build %q, body %q", build, body)
	}
	old, err := s.Get(ctx, "2026.09.02", firstBuild)
	if err != nil || old.Status != StatusReady {
		t.Fatalf("the previous target must stay stored: %+v, %v", old, err)
	}
	listedTargets := 0
	all, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range all {
		if rel.IsTarget {
			listedTargets++
			if rel.Build != rebuiltBuild {
				t.Fatalf("the list marks build %q as target", rel.Build)
			}
		}
	}
	if len(all) != 2 || listedTargets != 1 {
		t.Fatalf("want both builds listed and one target, got %d rows, %d targets", len(all), listedTargets)
	}
}

// TestAFailedPullOfANewBuildLeavesTheStoredOne: a newer build that cannot be
// fetched is recorded as its own failed row; the stored build is untouched.
func TestAFailedPullOfANewBuildLeavesTheStoredOne(t *testing.T) {
	s, rs := storeWithRebuild(t)
	ctx := t.Context()
	// The index announces a newer build whose files were never uploaded.
	rs.put("/m/latest.json", []byte(`{"version":"2026.09.02","build":"202610020900"}`))
	if _, err := s.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := s.Pull(ctx, "2026.09.02", "", "tester"); err == nil {
		t.Fatal("a pull with nothing to fetch must report its failure")
	}
	failed, err := s.Get(ctx, "2026.09.02", "202610020900")
	if err != nil || failed.Status != StatusFailed || failed.LastError == "" {
		t.Fatalf("the failed build is recorded on its own row: %+v, %v", failed, err)
	}
	stored, err := s.Get(ctx, "2026.09.02", firstBuild)
	if err != nil || stored.Status != StatusReady || stored.LastError != "" {
		t.Fatalf("the stored build must be untouched: %+v, %v", stored, err)
	}
	if build, body := servedBinary(t, s); build != firstBuild || body != "linux-amd64 agent 2026.09.02" {
		t.Fatalf("the old binary must still be served: build %q, body %q", build, body)
	}
}
