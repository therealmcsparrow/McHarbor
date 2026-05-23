package stacks

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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

func TestBuildSelfComposeHelperScriptIncludesLoggingAndRecovery(t *testing.T) {
	script := buildSelfComposeHelperScript("mcharbor-compose-helper-test", "mcharbor", []string{
		"docker compose -f docker-compose.yml pull",
		"docker compose -f docker-compose.yml up -d",
	})

	for _, want := range []string{
		"set -eu",
		"/app/data/self-update/mcharbor-compose-helper-test.log",
		"trap recover ERR",
		"docker compose -f docker-compose.yml pull",
		"docker compose -f docker-compose.yml up -d",
		"docker start 'mcharbor'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("helper script missing %q:\n%s", want, script)
		}
	}
}

func TestHelperWorkingDirNormalizesRelativeStackPath(t *testing.T) {
	got := helperWorkingDir("data/stacks/mcharbor")
	want := "/app/data/stacks/mcharbor"
	if got != want {
		t.Fatalf("helperWorkingDir() = %q, want %q", got, want)
	}
}
