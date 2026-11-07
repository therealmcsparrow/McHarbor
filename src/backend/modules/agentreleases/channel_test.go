// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

const (
	preVersion    = "2099.01.01-alpha.1"
	stableVersion = "2098.12.01"
)

func switchAgents(ctx context.Context, t *testing.T, s *Store, on, auto bool) {
	t.Helper()
	if _, _, err := updatechannel.Apply(ctx, s.db, updatechannel.Patch{AgentPrereleases: &on, AgentAutoPrereleases: &auto}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChannelValidVersion(t *testing.T) {
	good := []string{"2026.11.01", "2026.11.02-alpha.1", "2026.11.02-rc1", "1.2.3-0.beta.7"}
	bad := []string{
		"", "2026.11.01-", "2026.11.01-.", "2026.11.01-a..b", "2026.11.01-..", "2026.11.01-../x",
		"2026.11.01-a/b", `2026.11.01-a\b`, "2026.11.01-a_b", "2026.11.01-a-b", "2026.11.01+build",
		"2026.11.01-alpha\n", "2026.11.01 -alpha", "../2026.11.01", "2026.11.01-" + strings.Repeat("a", 200),
		"2026.11-alpha", "latest",
	}
	for _, v := range good {
		if !ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = false, want true", v)
		}
	}
	for _, v := range bad {
		if ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = true, want false", v)
		}
	}
}

func TestAgentChannelNewerOrdersPrereleases(t *testing.T) {
	cases := []struct {
		v1, b1, v2, b2 string
		want           bool
	}{
		{"2026.11.01", "", "2026.11.01-alpha.1", "", true},
		{"2026.11.01-alpha.1", "", "2026.11.01", "", false},
		{"2026.11.01-alpha.2", "", "2026.11.01-alpha.1", "", true},
		{"2026.11.01-alpha.10", "", "2026.11.01-alpha.9", "", true},
		{"2026.11.02-alpha.1", "", "2026.11.01", "", true},
		{"2026.11.01-alpha.1", "202611071300", "2026.11.01-alpha.1", "202611071200", true},
	}
	for _, tc := range cases {
		if got := newer(tc.v1, tc.b1, tc.v2, tc.b2); got != tc.want {
			t.Errorf("newer(%s %s, %s %s) = %v, want %v", tc.v1, tc.b1, tc.v2, tc.b2, got, tc.want)
		}
	}
}

func TestAgentChannel(t *testing.T) {
	pub, priv := newKey(t)
	_, otherPriv := newKey(t)

	latest := `{"version":"` + preVersion + `","versions":[` +
		`{"version":"` + preVersion + `"},{"version":"` + stableVersion + `"}]}`
	setup := func(t *testing.T, mutate func(f releaseFiles)) (*Store, context.Context) {
		t.Helper()
		ctx := context.Background()
		rs := newReleaseServer(t)
		for _, v := range []string{preVersion, stableVersion} {
			files := buildRelease(t, priv, v)
			if mutate != nil && v == preVersion {
				mutate(files)
			}
			for name, body := range files {
				rs.put("/m/"+v+"/"+name, body)
			}
		}
		rs.put("/m/latest.json", []byte(latest))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
		return s, ctx
	}

	t.Run("switch off: latest.json prerelease is skipped for the newest stable", func(t *testing.T) {
		s, ctx := setup(t, nil)
		st, err := s.Check(ctx)
		if err != nil || st.LatestVersion != stableVersion {
			t.Fatalf("check = %+v, %v", st, err)
		}
		for _, a := range st.Available {
			if strings.Contains(a.Version, "-") {
				t.Fatalf("available lists a prerelease with the switch off: %+v", st.Available)
			}
		}
	})

	t.Run("switch off and no stable listed: nothing is offered", func(t *testing.T) {
		ctx := context.Background()
		rs := newReleaseServer(t)
		rs.put("/m/latest.json", []byte(`{"version":"`+preVersion+`"}`))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
		st, err := s.Check(ctx)
		if err != nil || st.LatestVersion != "" || len(st.Available) != 0 {
			t.Fatalf("check = %+v, %v", st, err)
		}
	})

	t.Run("switch on: the prerelease is the latest", func(t *testing.T) {
		s, ctx := setup(t, nil)
		switchAgents(ctx, t, s, true, false)
		st, err := s.Check(ctx)
		if err != nil || st.LatestVersion != preVersion {
			t.Fatalf("check = %+v, %v", st, err)
		}
	})

	t.Run("switch off: pull and target of a prerelease are refused", func(t *testing.T) {
		s, ctx := setup(t, nil)
		if _, err := s.Pull(ctx, preVersion, "", "tester"); !errors.Is(err, ErrPrereleaseOff) {
			t.Fatalf("pull = %v, want ErrPrereleaseOff", err)
		}
		if err := s.StartPull(ctx, preVersion, "", "tester", true); !errors.Is(err, ErrPrereleaseOff) {
			t.Fatalf("start pull = %v, want ErrPrereleaseOff", err)
		}
		if _, err := s.Get(ctx, preVersion, ""); err == nil {
			t.Fatal("a refused pull left a row behind")
		}
	})

	t.Run("switch turned off: the saved check result stops offering the prerelease on read", func(t *testing.T) {
		s, ctx := setup(t, nil)
		switchAgents(ctx, t, s, true, false)
		if st, err := s.Check(ctx); err != nil || st.LatestVersion != preVersion {
			t.Fatalf("check = %+v, %v", st, err)
		}
		switchAgents(ctx, t, s, false, false)
		ov, err := s.Overview(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ov.State.LatestVersion != stableVersion {
			t.Fatalf("latest = %q, want the newest stable %q", ov.State.LatestVersion, stableVersion)
		}
		for _, a := range ov.State.Available {
			if strings.Contains(a.Version, "-") {
				t.Fatalf("available still lists a prerelease: %+v", ov.State.Available)
			}
		}
	})

	t.Run("a target stored while on stops being served when the switch goes off", func(t *testing.T) {
		s, ctx := setup(t, nil)
		switchAgents(ctx, t, s, true, false)
		if _, err := s.Pull(ctx, preVersion, "", "tester"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetTarget(ctx, preVersion, ""); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.TargetRelease(ctx); !ok {
			t.Fatal("target not served with the switch on")
		}
		switchAgents(ctx, t, s, false, false)
		if _, ok := s.TargetRelease(ctx); ok {
			t.Fatal("a prerelease target is still served with the switch off")
		}
		if err := s.SetTarget(ctx, preVersion, ""); !errors.Is(err, ErrPrereleaseOff) {
			t.Fatalf("set target = %v, want ErrPrereleaseOff", err)
		}
	})

	t.Run("a bad signature on a prerelease is refused as on stable", func(t *testing.T) {
		s, ctx := setup(t, func(f releaseFiles) { f.resign(otherPriv) })
		switchAgents(ctx, t, s, true, false)
		_, err := s.Pull(ctx, preVersion, "", "tester")
		if err == nil || !strings.Contains(err.Error(), ErrSignatureInvalid.Error()) {
			t.Fatalf("pull = %v, want a signature refusal", err)
		}
		if err := s.SetTarget(ctx, preVersion, ""); !errors.Is(err, ErrReleaseNotReady) {
			t.Fatalf("a refused prerelease became the target: %v", err)
		}
	})

	t.Run("rollout needs the auto flag to target a prerelease", func(t *testing.T) {
		s, ctx := setup(t, nil)
		cfg, err := s.Settings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Mode = ModeRollout
		if _, err := s.SaveSettings(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		switchAgents(ctx, t, s, true, false)
		if err := s.CheckAndAct(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Target(ctx); err != nil || got != "" {
			t.Fatalf("target = %q, %v, a prerelease was rolled out without the auto flag", got, err)
		}
		if rel, err := s.Get(ctx, preVersion, ""); err != nil || rel.Status != StatusReady {
			t.Fatalf("prerelease should still be downloaded: %+v, %v", rel, err)
		}
		switchAgents(ctx, t, s, true, true)
		if err := s.CheckAndAct(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Target(ctx); err != nil || got != preVersion {
			t.Fatalf("target = %q, %v, want %q with the auto flag on", got, err, preVersion)
		}
	})
}
