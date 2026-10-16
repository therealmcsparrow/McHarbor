// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// TestClassifyCatchesRenamedAndMirroredMcHarbor is M6: detection by three hard-coded image
// names, the name "mcharbor" and the compose labels let a retagged image, an HA peer or a
// renamed own container through.
func TestClassifyCatchesRenamedAndMirroredMcHarbor(t *testing.T) {
	SetSelfContainerIDs([]string{"abcdef012345"})
	t.Cleanup(func() { SetSelfContainerIDs(nil) })

	cases := []struct {
		name string
		info container.InspectResponse
		want SelfTarget
	}{
		{"role label on a retagged image", inspect("/prod-app", "registry.local/ops/harbor:9", map[string]string{AppRoleLabel: "app"}, nil), SelfTargetApp},
		{"HA peer compose service", inspect("/x", "custom:1", map[string]string{composeServiceLabel: "mcharbor-node-2", composeProjectLabel: "prod"}, nil), SelfTargetApp},
		{"HA peer container name", inspect("/prod-mcharbor-node-1-1", "custom:1", nil, nil), SelfTargetApp},
		{"registry mirror", inspect("/web", "registry.local:5000/mirror/mcharbor:2026.09.01", nil, nil), SelfTargetApp},
		{"older image by its env", inspect("/web", "retagged:1", nil, []string{"MCHARBOR_PLUGIN_TRUST_KEYS=/app/trust/plugin-signing.pub"}), SelfTargetApp},
		{"own container renamed", withID(inspect("/renamed", "whatever:1", nil, nil), "abcdef0123456789"), SelfTargetApp},
		{"self-update helper", inspect("/mcharbor-compose-helper-x", "busybox", map[string]string{HelperLabel: "self-update"}, nil), SelfTargetHelper},
		{"operator protected", inspect("/db", "postgres:17", map[string]string{ProtectedLabel: "true"}, nil), SelfTargetProtected},
		{"the agent is not the app", inspect("/mcharbor-agent", "ghcr.io/therealmcsparrow/mcharbor-agent:latest", nil, nil), SelfTargetNone},
		{"an ordinary container", inspect("/nginx", "nginx:1.27", nil, nil), SelfTargetNone},
	}
	for _, tc := range cases {
		if got := ClassifyContainer(tc.info); got != tc.want {
			t.Errorf("%s: ClassifyContainer = %q, want %q", tc.name, got, tc.want)
		}
	}
	if !IsProtectedContainer([]string{"/prod-mcharbor-node-1-1"}, "custom:1", nil) {
		t.Error("IsProtectedContainer misses an HA peer")
	}
	if got := ClassifySummary("abcdef0123456789", []string{"/renamed"}, "whatever:1", nil); got != SelfTargetApp {
		t.Errorf("ClassifySummary of the renamed own container = %q, want app", got)
	}
}

func inspect(name, image string, labels map[string]string, env []string) container.InspectResponse {
	return container.InspectResponse{Name: name, Config: &container.Config{Image: image, Labels: labels, Env: env}}
}

func withID(info container.InspectResponse, id string) container.InspectResponse {
	info.ID = id
	return info
}

func TestHostConfigReasons(t *testing.T) {
	host := SelfHost{Hosted: true, Name: "mcharbor", ContainerID: "bbbbbbbbbbbb0002", Paths: []string{"/srv/mcharbor/data"}, Volumes: []string{"mcharbor_mcharbor-data"}}
	cases := []struct {
		name string
		hc   *container.HostConfig
		want []string
	}{
		{"socket bind", &container.HostConfig{Binds: []string{"/var/run/docker.sock:/var/run/docker.sock"}}, []string{HostAccessDockerSocket}},
		{"directory holding the socket", &container.HostConfig{Binds: []string{"/run:/r:ro"}}, []string{HostAccessDockerSocket}},
		{"host root", &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: "/", Target: "/host"}}}, []string{HostAccessHostRoot}},
		{"inside /proc", &container.HostConfig{Binds: []string{"/proc/1/root:/h"}}, []string{HostAccessHostProc}},
		{"/etc", &container.HostConfig{Binds: []string{"/etc:/e"}}, []string{HostAccessHostEtc}},
		{"McHarbor's data dir", &container.HostConfig{Binds: []string{"/srv/mcharbor/data/mcharbor.db:/x"}}, []string{HostAccessOwnData}},
		{"McHarbor's volume", &container.HostConfig{Binds: []string{"mcharbor_mcharbor-data:/x"}}, []string{HostAccessOwnVolume}},
		{"volumes_from McHarbor", &container.HostConfig{VolumesFrom: []string{"mcharbor:ro"}}, []string{HostAccessVolumesFrom}},
		{"privileged, pid host, ALL caps", &container.HostConfig{Privileged: true, PidMode: "host", CapAdd: []string{"ALL"}}, []string{HostAccessPrivileged, HostAccessPIDHost, HostAccessSysAdmin}},
		{"ordinary data bind", &container.HostConfig{Binds: []string{"/srv/app:/data", "appdata:/cache"}}, nil},
	}
	for _, tc := range cases {
		if got := HostConfigReasons(tc.hc, host); !slices.Equal(got, tc.want) {
			t.Errorf("%s: reasons = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestComposeHostAccessReasons(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evil")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("HOSTROOT=/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := SelfHost{Hosted: true, ComposeProject: "mcharbor", Volumes: []string{"mcharbor_mcharbor-data"}}

	compose := `
services:
  sock:
    image: docker:cli
    volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
  root:
    image: alpine
    volumes:
      - ${HOSTROOT:-/srv}:/host
  priv:
    image: alpine
    privileged: "true"
    pid: host
  steal:
    image: alpine
    volumes:
      - type: volume
        source: data
        target: /x
  plain:
    image: nginx
    volumes: ["./html:/usr/share/nginx/html", "cache:/cache"]
volumes:
  data:
    external: true
    name: mcharbor_mcharbor-data
  cache: {}
`
	got, err := ComposeHostAccessReasons([]byte(compose), dir, nil, host)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"sock":  {HostAccessDockerSocket},
		"root":  {HostAccessHostRoot},
		"priv":  {HostAccessPrivileged, HostAccessPIDHost},
		"steal": {HostAccessOwnVolume},
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %+v, want services %v", got, want)
	}
	for _, f := range got {
		if !slices.Equal(f.Reasons, want[f.Service]) {
			t.Errorf("%s: reasons %v, want %v", f.Service, f.Reasons, want[f.Service])
		}
	}

	takeover, err := ComposeHostAccessReasons([]byte("name: mcharbor\nservices:\n  mcharbor:\n    image: evil/img\n"), dir, nil, host)
	if err != nil {
		t.Fatal(err)
	}
	if len(takeover) != 1 || takeover[0].Service != "" || !slices.Contains(takeover[0].Reasons, HostAccessOwnProject) {
		t.Errorf("a compose file named after McHarbor's project was not flagged: %+v", takeover)
	}

	escape, err := ComposeHostAccessReasons([]byte("services:\n  a:\n    image: x\n    volumes: [\"../../../../../..:/h\"]\n"), "/app/data/stacks/evil", nil, host)
	if err != nil {
		t.Fatal(err)
	}
	if len(escape) != 1 || !slices.Contains(escape[0].Reasons, HostAccessHostRoot) {
		t.Errorf("a relative path climbing to / was not flagged: %+v", escape)
	}
}
