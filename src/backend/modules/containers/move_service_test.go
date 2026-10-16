// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestNormalizedMoveName(t *testing.T) {
	tests := []struct {
		name     string
		fallback string
		want     string
	}{
		{name: "/web", fallback: "abc123", want: "web"},
		{name: " api ", fallback: "abc123", want: "api"},
		{name: "", fallback: "/fallback", want: "fallback"},
	}

	for _, test := range tests {
		if got := normalizedMoveName(test.name, test.fallback); got != test.want {
			t.Fatalf("normalizedMoveName(%q, %q) = %q, want %q", test.name, test.fallback, got, test.want)
		}
	}
}

func TestMoveImageReferencePrefersConfiguredImage(t *testing.T) {
	info := container.InspectResponse{
		Image:  "sha256:abc",
		Config: &container.Config{Image: "nginx:1.27"},
	}

	if got := moveImageReference(info); got != "nginx:1.27" {
		t.Fatalf("moveImageReference() = %q, want configured image", got)
	}
}

func TestMoveWarningsRunningContainer(t *testing.T) {
	warnings := moveWarnings(container.InspectResponse{
		State:      &container.State{Running: true},
		HostConfig: &container.HostConfig{},
	})

	if len(warnings) == 0 {
		t.Fatal("expected warning for running source container")
	}
}
