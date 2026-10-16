// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"testing"
)

const (
	pinOldBuild = "202610050818"
	pinNewBuild = "202610060104"
)

// buildOf is a signed release of version whose BUILD file says build.
func buildOf(priv ed25519.PrivateKey, version, build string) releaseFiles {
	files := releaseFiles{
		"mcharbor-agent-linux-amd64":       []byte("linux-amd64 agent " + version + " " + build),
		"mcharbor-agent-windows-amd64.exe": []byte("windows agent " + version + " " + build),
		"BUILD":                            []byte(build + "\n"),
	}
	files.resign(priv)
	return files
}

// twoBuildCatalog publishes two builds of one version under an index file, the
// way the mcharbor.eu agent catalog lays them out.
func twoBuildCatalog(t *testing.T, priv ed25519.PrivateKey) *releaseServer {
	t.Helper()
	rs := newReleaseServer(t)
	for _, build := range []string{pinOldBuild, pinNewBuild} {
		for name, body := range buildOf(priv, "2026.10.01", build) {
			rs.put("/cat/2026.10.01/"+build+"/"+name, body)
		}
	}
	rs.put("/cat/index.json", fmt.Appendf(nil,
		`{"version":"2026.10.01","build":%q,"versions":[{"version":"2026.10.01","build":%q},{"version":"2026.10.01","build":%q}]}`,
		pinNewBuild, pinNewBuild, pinOldBuild))
	return rs
}

func listed(t *testing.T, s *Store, version string) Release {
	t.Helper()
	all, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range all {
		if rel.Version == version {
			return rel
		}
	}
	t.Fatalf("version %s not stored", version)
	return Release{}
}

// TestPullingAnOlderBuildPinsIt: an operator who steps back to an older build keeps
// it, even though the newest build is listed and the scheduler pulls the version.
func TestPullingAnOlderBuildPinsIt(t *testing.T) {
	pub, priv := newKey(t)
	rs := twoBuildCatalog(t, priv)
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/cat/index.json"}}, storeOpts{})
	ctx := t.Context()
	if _, err := s.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}

	rel, err := s.Pull(ctx, "2026.10.01", pinOldBuild, "tester")
	if err != nil || rel.Build != pinOldBuild {
		t.Fatalf("pull of the older build: %+v, %v", rel, err)
	}
	if !listed(t, s, "2026.10.01").Pinned {
		t.Fatal("an older build chosen over a listed newer one is not marked pinned")
	}

	// The scheduler's call: no build named. A pinned build must stand.
	if rel, err := s.Pull(ctx, "2026.10.01", "", "scheduler"); err != nil || rel.Build != pinOldBuild {
		t.Fatalf("the automatic pull rolled the pinned build forward: %+v, %v", rel, err)
	}
	if rs.hitsUnder("/cat/2026.10.01/"+pinNewBuild+"/") != 0 {
		t.Fatal("the newer build was fetched while the older one was pinned")
	}

	rel, err = s.Pull(ctx, "2026.10.01", pinNewBuild, "tester")
	if err != nil || rel.Build != pinNewBuild {
		t.Fatalf("pull of the newest build: %+v, %v", rel, err)
	}
	if listed(t, s, "2026.10.01").Pinned {
		t.Fatal("choosing the newest build again left the version pinned")
	}
}

// TestPullingAnUnpublishedBuildChangesNothing: a named build has one home, and a
// source without it must not hand over whatever build it does have.
func TestPullingAnUnpublishedBuildChangesNothing(t *testing.T) {
	pub, priv := newKey(t)
	rs := twoBuildCatalog(t, priv)
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/cat/index.json"}}, storeOpts{})
	ctx := t.Context()
	if _, err := s.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := s.Pull(ctx, "2026.10.01", pinNewBuild, "tester"); err != nil {
		t.Fatalf("first pull: %v", err)
	}

	if _, err := s.Pull(ctx, "2026.10.01", "202601010000", "tester"); err == nil {
		t.Fatal("a build no source publishes was reported as pulled")
	}
	if rel := listed(t, s, "2026.10.01"); rel.Build != pinNewBuild || rel.Status != StatusReady {
		t.Fatalf("a failed pull of another build disturbed the stored one: %+v", rel)
	}

	if _, err := s.Pull(ctx, "2026.10.01", "not-a-build", "tester"); !errors.Is(err, ErrInvalidBuild) {
		t.Fatalf("malformed build: %v, want ErrInvalidBuild", err)
	}
}

// TestPullRefusesASourceServingAnotherBuild: the signed BUILD file is what
// identifies a release, so a folder holding a different build is refused.
func TestPullRefusesASourceServingAnotherBuild(t *testing.T) {
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	for name, body := range buildOf(priv, "2026.10.01", pinNewBuild) {
		rs.put("/cat/2026.10.01/"+pinOldBuild+"/"+name, body)
	}
	rs.put("/cat/index.json", fmt.Appendf(nil, `{"version":"2026.10.01","build":%q}`, pinOldBuild))
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/cat/index.json"}}, storeOpts{})

	_, err := s.Pull(t.Context(), "2026.10.01", pinOldBuild, "tester")
	if err == nil || !errors.Is(err, ErrNoSourceAvailable) {
		t.Fatalf("a folder serving another build: %v, want a refusal", err)
	}
}
