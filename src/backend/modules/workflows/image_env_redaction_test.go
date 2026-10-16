// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"slices"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// TestImageInspectNodeWithholdsEnvCredentials: the image-inspect node's output is stored with
// the run and readable by every workflow viewer, so it carries the masked env, never the values.
func TestImageInspectNodeWithholdsEnvCredentials(t *testing.T) {
	env := []string{"PATH=/usr/bin", "API_TOKEN=tok-abcdef", "DATABASE_URL=postgres://app:pw123456@db/app"}
	got := imageEnvForRun(env)
	want := []string{"PATH=/usr/bin", "API_TOKEN=" + envredact.Marker, "DATABASE_URL=" + envredact.Marker}
	if !slices.Equal(got, want) {
		t.Fatalf("image env in a run = %q\nwant              %q", got, want)
	}
	if env[1] != "API_TOKEN=tok-abcdef" {
		t.Fatalf("the inspect result was modified in place: %q", env)
	}
}
