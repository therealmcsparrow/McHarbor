// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func TestIsSelfMcHarborContainerByName(t *testing.T) {
	info := types.ContainerJSON{
		ContainerJSONBase: &container.ContainerJSONBase{Name: "/mcharbor"},
		Config:            &container.Config{Image: "ghcr.io/therealmcsparrow/mcharbor:latest"},
	}

	if !isSelfMcHarborContainer(info) {
		t.Fatal("expected mcharbor container name to be detected")
	}
}

func TestIsSelfMcHarborContainerDoesNotMatchAgent(t *testing.T) {
	info := types.ContainerJSON{
		ContainerJSONBase: &container.ContainerJSONBase{Name: "/mcharbor-agent"},
		Config:            &container.Config{Image: "ghcr.io/therealmcsparrow/mcharbor-agent:latest"},
	}

	if isSelfMcHarborContainer(info) {
		t.Fatal("expected mcharbor-agent not to be detected as the app container")
	}
}
