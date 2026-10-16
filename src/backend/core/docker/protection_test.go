// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestIsMcHarborContainerDoesNotMatchGenericProtectedLabel(t *testing.T) {
	if IsMcHarborContainer([]string{"/app"}, "nginx:latest", map[string]string{ProtectedLabel: "true"}) {
		t.Fatal("expected generic protected label not to identify the McHarbor app container")
	}
}

func TestIsProtectedContainerStillMatchesProtectedLabel(t *testing.T) {
	if !IsProtectedContainer([]string{"/app"}, "nginx:latest", map[string]string{ProtectedLabel: "true"}) {
		t.Fatal("expected protected label to lock the container")
	}
}

func TestIsMcHarborContainerMatchesComposeImageAndName(t *testing.T) {
	if !IsMcHarborContainer([]string{"/app"}, "nginx:latest", map[string]string{
		composeProjectLabel: mcHarborProject,
		composeServiceLabel: mcHarborService,
	}) {
		t.Fatal("expected McHarbor compose labels to identify the app container")
	}
	if !IsMcHarborContainer([]string{"/app"}, "ghcr.io/therealmcsparrow/mcharbor:latest", nil) {
		t.Fatal("expected McHarbor image to identify the app container")
	}
	if !IsMcHarborContainer([]string{"/mcharbor-mcharbor-1"}, "nginx:latest", nil) {
		t.Fatal("expected McHarbor compose container name to identify the app container")
	}
}

func TestSelfUpdateTargetImageUsesRequestedImageWhenProvided(t *testing.T) {
	current := container.InspectResponse{Config: &container.Config{Image: "ghcr.io/therealmcsparrow/mcharbor:old"}}
	if got := selfUpdateTargetImage(current, "ghcr.io/therealmcsparrow/mcharbor:new"); got != "ghcr.io/therealmcsparrow/mcharbor:new" {
		t.Fatalf("expected requested image, got %q", got)
	}
	if got := selfUpdateTargetImage(current, ""); got != "ghcr.io/therealmcsparrow/mcharbor:old" {
		t.Fatalf("expected current image fallback, got %q", got)
	}
}
