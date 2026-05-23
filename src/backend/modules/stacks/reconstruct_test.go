package stacks

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

func TestReconstructComposeSkipsGeneratedHostname(t *testing.T) {
	containerID := "1f386cd87629916ec1e1501d41f44f86b00cc4b18f5080e353b30e26f26cf4d7"

	compose, err := ReconstructCompose([]types.ContainerJSON{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID: containerID,
			},
			Config: &container.Config{
				Hostname: containerID[:12],
				Image:    "ghcr.io/therealmcsparrow/mcharbor:1.1.4",
				Labels: map[string]string{
					"com.docker.compose.service": "mcharbor",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ReconstructCompose() error = %v", err)
	}

	if strings.Contains(compose, "hostname:") {
		t.Fatalf("generated compose should not include Docker-generated hostname:\n%s", compose)
	}
}

func TestContainerMatchesAnyCandidateWithStaleHostnameAndRealID(t *testing.T) {
	containerID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	candidates := []string{
		"old-hostname",
		containerID,
	}

	if !containerMatchesAnyCandidate(containerID, []string{"/mcharbor"}, candidates) {
		t.Fatal("expected real container ID candidate to match even when hostname candidate is stale")
	}
}

func TestCloneSelfContainerConfigUsesTargetImageAndDropsGeneratedHostname(t *testing.T) {
	current := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID: "49ca1252ea2939a2168b02a7ff0e4a2a2d020da55ad596a4ece4037e7d8f1f82",
			HostConfig: &container.HostConfig{
				AutoRemove: true,
			},
		},
		Config: &container.Config{
			Hostname: "49ca1252ea29",
			Image:    "ghcr.io/therealmcsparrow/mcharbor:1.1.6",
		},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"mcharbor_default": {
					Aliases: []string{"mcharbor", "49ca1252ea29", "mcharbor"},
				},
			},
		},
	}

	cfg, hostCfg, netCfg := cloneSelfContainerConfig(current, "ghcr.io/therealmcsparrow/mcharbor:1.1.7")
	if cfg.Image != "ghcr.io/therealmcsparrow/mcharbor:1.1.7" {
		t.Fatalf("cfg.Image = %q", cfg.Image)
	}
	if cfg.Hostname != "" {
		t.Fatalf("generated hostname should be cleared, got %q", cfg.Hostname)
	}
	if hostCfg.AutoRemove {
		t.Fatal("replacement McHarbor container must not be auto-remove")
	}
	aliases := netCfg.EndpointsConfig["mcharbor_default"].Aliases
	if len(aliases) != 1 || aliases[0] != "mcharbor" {
		t.Fatalf("aliases = %#v", aliases)
	}
}

func TestSelfContainerMatchesManagedStackByComposeWorkingDir(t *testing.T) {
	current := types.ContainerJSON{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name: "/mcharbor",
		},
		Config: &container.Config{
			Labels: map[string]string{
				"com.docker.compose.project":             "docker",
				"com.docker.compose.project.working_dir": "/opt/mcharbor",
			},
		},
	}
	stack := &Stack{
		Name:        "mcharbor",
		ProjectPath: "/opt/mcharbor",
		ComposeFile: "docker-compose.yml",
	}

	if !selfContainerMatchesManagedStack(current, stack) {
		t.Fatal("expected current container to match managed stack by compose working directory")
	}
}

func TestComposeReferencesContainerName(t *testing.T) {
	compose := `
services:
  app:
    image: ghcr.io/therealmcsparrow/mcharbor:1.1.9
    container_name: "mcharbor"
`

	if !composeReferencesContainerName(compose, "mcharbor") {
		t.Fatal("expected compose container_name to match")
	}
	if composeReferencesContainerName(compose, "other") {
		t.Fatal("unexpected compose container_name match")
	}
}
