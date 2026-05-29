// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

func TestIsSelfMcHarborContainerByName(t *testing.T) {
	if !coredocker.IsProtectedContainer([]string{"/mcharbor"}, "ghcr.io/therealmcsparrow/mcharbor:latest", nil) {
		t.Fatal("expected mcharbor container name to be detected")
	}
}

func TestIsSelfMcHarborContainerDoesNotMatchAgent(t *testing.T) {
	if coredocker.IsProtectedContainer([]string{"/mcharbor-agent"}, "ghcr.io/therealmcsparrow/mcharbor-agent:latest", nil) {
		t.Fatal("expected mcharbor-agent not to be detected as the app container")
	}
}
