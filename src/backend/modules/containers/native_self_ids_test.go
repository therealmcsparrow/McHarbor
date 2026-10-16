// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// TestNativeHostHasNoSelfContainer guards the native install: the self-id detection reads every
// running container's id out of a host's mount table, which locked ordinary containers as
// "McHarbor's own". main sets an empty override on a native host; with it nothing is self.
func TestNativeHostHasNoSelfContainer(t *testing.T) {
	t.Cleanup(func() { docker.SetSelfContainerIDs(nil) })
	const id = "3f2a9c41b7d05e6a8c1f4b2d9e7a6c5b4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a"
	names := []string{"/homeassistant"}
	image := "ghcr.io/home-assistant/home-assistant:stable"

	// What the mountinfo scan produced on a native host: the container's own id.
	docker.SetSelfContainerIDs([]string{id[:12]})
	if got := docker.ClassifySummary(id, names, image, nil); got != docker.SelfTargetApp {
		t.Fatalf("precondition: a listed id should classify as self, got %q", got)
	}

	docker.SetSelfContainerIDs([]string{})
	if got := docker.ClassifySummary(id, names, image, nil); got != docker.SelfTargetNone {
		t.Errorf("an ordinary container on a native host is %q, want unlocked", got)
	}
}
