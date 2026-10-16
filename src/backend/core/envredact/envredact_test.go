// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package envredact

import (
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
)

// TestViewerSeesKeysButNotCredentials is the default every env surface relies on: the names
// stay readable, the credential-looking values do not.
func TestViewerSeesKeysButNotCredentials(t *testing.T) {
	env := []string{
		"PATH=/usr/bin", "DB_PASSWORD=hunter2hunter2", "API_TOKEN=tok-123456",
		"DATABASE_URL=postgres://app:s3cretpw@db:5432/app", "AUTH_DISABLE=false", "EMPTY_SECRET=",
	}
	got, withheld := Policy{}.EnvList(env)
	want := []string{
		"PATH=/usr/bin", "DB_PASSWORD=" + Marker, "API_TOKEN=" + Marker,
		"DATABASE_URL=" + Marker, "AUTH_DISABLE=false", "EMPTY_SECRET=",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("EnvList = %q\nwant      %q", got, want)
	}
	if !slices.Equal(withheld, []string{"DB_PASSWORD", "API_TOKEN", "DATABASE_URL"}) {
		t.Errorf("withheld = %q", withheld)
	}
	if env[1] != "DB_PASSWORD=hunter2hunter2" {
		t.Error("EnvList modified its input")
	}
}

// TestRevealNeverReachesTheRootOfTrust guards the one rule a permission cannot override: a
// McHarbor container's own keys, and any value carrying a key this process runs with.
func TestRevealNeverReachesTheRootOfTrust(t *testing.T) {
	const secret = "pinned-mcharbor-secret-value"
	p := Policy{Reveal: true, McHarbor: true, Pinned: PinnedValues(secret, "short")}
	got, _ := p.EnvList([]string{
		"MCHARBOR_SECRET=anything-at-all", "ENCRYPTION_KEY=k", "MCHARBOR_DB_DSN=postgres://x",
		"COPIED=prefix-" + secret, "DB_PASSWORD=hunter2hunter2",
	})
	want := []string{
		"MCHARBOR_SECRET=" + Marker, "ENCRYPTION_KEY=" + Marker, "MCHARBOR_DB_DSN=" + Marker,
		"COPIED=" + Marker, "DB_PASSWORD=hunter2hunter2",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("reveal on a McHarbor container = %q\nwant %q", got, want)
	}

	other := Policy{Reveal: true, Pinned: PinnedValues(secret)}
	if got, _ := other.EnvList([]string{"ENCRYPTION_KEY=app-own-key"}); got[0] != "ENCRYPTION_KEY=app-own-key" {
		t.Errorf("another app's ENCRYPTION_KEY was withheld from a permitted reveal: %q", got[0])
	}
	if len(PinnedValues("", "short", "  ")) != 0 {
		t.Error("a short or empty pinned value would match unrelated text")
	}
}

// TestContainerDoesNotAliasTheCallersConfig keeps a redacted response from rewriting a value
// the caller still needs, such as the recreate path's original environment.
func TestContainerDoesNotAliasTheCallersConfig(t *testing.T) {
	cfg := &container.Config{Env: []string{"DB_PASSWORD=hunter2hunter2"}}
	info := container.InspectResponse{Config: cfg}
	if withheld := (Policy{}).Container(&info); len(withheld) != 1 {
		t.Fatalf("withheld = %q", withheld)
	}
	if info.Config.Env[0] != "DB_PASSWORD="+Marker {
		t.Errorf("response env = %q", info.Config.Env)
	}
	if cfg.Env[0] != "DB_PASSWORD=hunter2hunter2" {
		t.Errorf("the original config was modified: %q", cfg.Env)
	}
}

// TestRestoreKeepsWhatTheEditorDidNotChange is the write-path half: a redacted list sent
// back must not replace a credential with the marker.
func TestRestoreKeepsWhatTheEditorDidNotChange(t *testing.T) {
	current := []string{"DB_PASSWORD=hunter2hunter2", "PLAIN=1", "TOKEN=old-token"}
	redacted, _ := Policy{}.EnvList(current)
	edited := append(slices.Clone(redacted), "ADDED="+Marker)
	edited[1] = "PLAIN=2"
	edited[2] = "TOKEN=typed-new-token"

	got := Restore(edited, current)
	want := []string{"DB_PASSWORD=hunter2hunter2", "PLAIN=2", "TOKEN=typed-new-token", "ADDED="}
	if !slices.Equal(got, want) {
		t.Fatalf("Restore = %q\nwant    %q", got, want)
	}
	if Restore(nil, current) != nil {
		t.Error("a request that did not touch the environment must stay nil")
	}
}
