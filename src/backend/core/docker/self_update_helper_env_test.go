// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"slices"
	"testing"
)

// TestHelperEnvLeavesOutTheRootOfTrust guards I-9: the helpers copied McHarbor's whole process
// environment into two more containers whose inspect output any viewer could read.
func TestHelperEnvLeavesOutTheRootOfTrust(t *testing.T) {
	got := helperBaseEnv([]string{
		"PATH=/usr/bin", "DOCKER_HOST=unix:///var/run/docker.sock", "ENCRYPTION_KEY=k",
		"MCHARBOR_SECRET=s", "MCHARBOR_DB_DSN=postgres://x", "ENCRYPTION_KEY_FILE=/run/secrets/k",
		"MCHARBOR_SECRET_FILE=/run/secrets/s", "TZ=UTC",
	})
	want := []string{"PATH=/usr/bin", "DOCKER_HOST=unix:///var/run/docker.sock", "TZ=UTC"}
	if !slices.Equal(got, want) {
		t.Fatalf("helper env = %q\nwant %q", got, want)
	}
}
