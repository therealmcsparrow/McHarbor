// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

func reasonsOf(t *testing.T, compose string, lookup VolumeLookup) map[string][]string {
	t.Helper()
	got, err := ComposeHostAccessReasonsWith([]byte(compose), "/app/data/stacks/probe", nil, SelfHost{Hosted: true}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, f := range got {
		out[f.Service] = f.Reasons
	}
	return out
}

func wantReasons(t *testing.T, got map[string][]string, want map[string][]string) {
	t.Helper()
	for svc, reasons := range want {
		for _, r := range reasons {
			if !slices.Contains(got[svc], r) {
				t.Errorf("service %q reasons %v; want %s", svc, got[svc], r)
			}
		}
	}
}

// TestNamespaceJoinsAreHostAccess is C1: a pid, ipc or network namespace joined from another
// container reaches McHarbor's /proc/<pid>/environ when that container is McHarbor.
func TestNamespaceJoinsAreHostAccess(t *testing.T) {
	for name, hc := range map[string]*container.HostConfig{
		"pid":  {PidMode: "container:mcharbor"},
		"ipc":  {IpcMode: "container:abcdef012345"},
		"net":  {NetworkMode: "container:mcharbor"},
		"uts":  {UTSMode: "container:mcharbor"},
		"svc":  {NetworkMode: "service:vpn"},
		"case": {PidMode: "Container:x"},
	} {
		if got := HostConfigReasons(hc, SelfHost{}); !slices.Contains(got, HostAccessJoinsNamespace) {
			t.Errorf("%s: reasons %v; want %s", name, got, HostAccessJoinsNamespace)
		}
	}
	if got := HostConfigReasons(&container.HostConfig{NetworkMode: "bridge", IpcMode: "shareable", PidMode: ""}, SelfHost{}); len(got) != 0 {
		t.Errorf("own namespaces: %v", got)
	}
	got := reasonsOf(t, `services:
  pid: {image: a, pid: "service:app"}
  ipc: {image: a, ipc: "container:mcharbor"}
  net: {image: a, network_mode: "service:vpn"}
  plain: {image: a, network_mode: bridge}
`, nil)
	wantReasons(t, got, map[string][]string{"pid": {HostAccessJoinsNamespace}, "ipc": {HostAccessJoinsNamespace}, "net": {HostAccessJoinsNamespace}})
	if len(got["plain"]) != 0 {
		t.Errorf("plain service: %v", got["plain"])
	}
}

// TestVolumesDisguisingABind is C2: a local volume of type none with o=bind is a bind of its
// device, in a container's Mounts, in compose driver_opts and in an existing named volume.
func TestVolumesDisguisingABind(t *testing.T) {
	bindOpts := map[string]string{"type": "none", "o": "bind", "device": "/"}
	hc := &container.HostConfig{Mounts: []mount.Mount{{
		Type: mount.TypeVolume, Source: "x", Target: "/h",
		VolumeOptions: &mount.VolumeOptions{DriverConfig: &mount.Driver{Name: "local", Options: bindOpts}},
	}}}
	if got := HostConfigReasons(hc, SelfHost{}); !slices.Contains(got, HostAccessHostRoot) {
		t.Errorf("Mounts volume with bind driver opts: %v; want host_root", got)
	}
	srv := &container.HostConfig{Mounts: []mount.Mount{{
		Type: mount.TypeVolume, Source: "y", Target: "/h",
		VolumeOptions: &mount.VolumeOptions{DriverConfig: &mount.Driver{Options: map[string]string{"o": "bind", "device": "/srv/x"}}},
	}}}
	if got := createBindReasons(srv, SelfHost{}); len(got) != 0 {
		t.Errorf("Mounts volume binding /srv/x at create: %v; want none", got)
	}
	srv.Mounts[0].VolumeOptions.DriverConfig.Options["device"] = "/var"
	if got := createBindReasons(srv, SelfHost{}); !slices.Contains(got, HostAccessDockerData) {
		t.Errorf("Mounts volume binding /var at create: %v; want docker_data", got)
	}
	if got := VolumeDeviceReasons("local", map[string]string{"type": "nfs", "o": "addr=10.0.0.1", "device": ":/export"}, SelfHost{}, true); len(got) != 0 {
		t.Errorf("an NFS volume is not a host path: %v", got)
	}
	if got := VolumeDeviceReasons("", map[string]string{"type": "ext4", "device": "/dev/sda1"}, SelfHost{}, true); !slices.Equal(got, []string{HostAccessDevices}) {
		t.Errorf("a block-device volume: %v; want devices", got)
	}

	got := reasonsOf(t, `services:
  top: {image: a, volumes: ["hostroot:/h"]}
  existing: {image: a, volumes: ["sneaky:/h"]}
  plain: {image: a, volumes: ["cache:/c"]}
volumes:
  hostroot:
    driver: local
    driver_opts: {type: none, o: bind, device: /etc}
  sneaky:
    external: true
  cache: {}
`, func(name string) (string, map[string]string, bool, error) {
		if name == "sneaky" {
			return "local", bindOpts, true, nil
		}
		return "", nil, false, nil
	})
	wantReasons(t, got, map[string][]string{"top": {HostAccessHostEtc}, "existing": {HostAccessHostRoot}})
	if len(got["plain"]) != 0 {
		t.Errorf("plain named volume: %v", got["plain"])
	}
}

// TestSensitivePathDenylist is H1: a bind of a denied root, anything under it or any parent
// of it is refused, after cleaning; data trees an operator or the App Store uses stay open.
func TestSensitivePathDenylist(t *testing.T) {
	host := SelfHost{Hosted: true, Paths: []string{"/srv/mcharbor/data"}}
	roots := map[string]string{
		"/proc": HostAccessHostProc, "/sys": HostAccessHostSys, "/dev": HostAccessHostDev,
		"/etc": HostAccessHostEtc, "/root": HostAccessHostHome, "/home": HostAccessHostHome,
		"/boot": HostAccessHostSystem, "/usr": HostAccessHostSystem, "/bin": HostAccessHostSystem,
		"/sbin": HostAccessHostSystem, "/lib": HostAccessHostSystem, "/lib32": HostAccessHostSystem,
		"/lib64": HostAccessHostSystem, "/libx32": HostAccessHostSystem,
		"/opt/containerd": HostAccessRuntimeData, "/var/lib/docker": HostAccessDockerData,
		"/var/lib/containerd": HostAccessRuntimeData, "/var/lib/kubelet": HostAccessRuntimeData,
		"/var/run": HostAccessDockerSocket, "/run": HostAccessDockerSocket,
		"/var/spool/cron": HostAccessHostCron, "/var/lib/mcharbor": HostAccessOwnData,
	}
	for root, reason := range roots {
		for _, src := range []string{root, root + "/", root + "/child/file", "/srv/../" + root + "/./x"} {
			if got := HostBindReasons(src, host); !slices.Contains(got, reason) {
				t.Errorf("%s: %v; want %s", src, got, reason)
			}
		}
	}
	parents := map[string][]string{
		"/":          {HostAccessHostRoot},
		"//":         {HostAccessHostRoot},
		"/srv/../":   {HostAccessHostRoot},
		"/var":       {HostAccessDockerSocket, HostAccessDockerData, HostAccessRuntimeData, HostAccessHostCron, HostAccessOwnData},
		"/var/lib/":  {HostAccessDockerData, HostAccessRuntimeData, HostAccessOwnData},
		"/var/spool": {HostAccessHostCron},
		"/opt":       {HostAccessRuntimeData},
		"/srv":       {HostAccessOwnData},
	}
	for src, want := range parents {
		got := HostBindReasons(src, host)
		for _, r := range want {
			if !slices.Contains(got, r) {
				t.Errorf("parent %s: %v; want %s", src, got, r)
			}
		}
	}
	for _, src := range []string{
		"/srv/media", "/srv/media/films", "/mnt/usb", "/media/disk", "/opt/appdata/jellyfin/config",
		"/opt/appdata", "/data/app", "/var/log/../../srv/x", "/usrlocal", "/homes/x", "/opt/containerdx",
	} {
		if got := HostBindReasons(src, host); len(got) != 0 {
			t.Errorf("%s is not sensitive: %v", src, got)
		}
	}
	got := reasonsOf(t, `services:
  var: {image: a, volumes: ["/var:/v"]}
  up: {image: a, volumes: ["../../../../etc:/e"]}
  own: {image: a, volumes: ["./html:/h", "/opt/appdata/probe/data:/data", "/srv/media:/m"]}
`, nil)
	wantReasons(t, got, map[string][]string{"var": {HostAccessDockerData}, "up": {HostAccessHostEtc}})
	if len(got["own"]) != 0 {
		t.Errorf("project-relative, app-data and /srv binds: %v", got["own"])
	}
}

// TestComposeFieldsTheCheckMissed is H2: secrets and configs with file:, a build context or
// env_file outside the project, device_cgroup_rules, cgroup: host and uts: host.
func TestComposeFieldsTheCheckMissed(t *testing.T) {
	got := reasonsOf(t, `services:
  secret: {image: a, secrets: [shadow]}
  config: {image: a, configs: [{source: keys, target: /k}]}
  ctxabs: {build: /}
  ctxup: {build: {context: ../..}}
  ctxok: {build: {context: ., dockerfile: Dockerfile}}
  envabs: {image: a, env_file: /proc/self/environ}
  envup: {image: a, env_file: [{path: ../../mcharbor.env, required: false}]}
  envok: {image: a, env_file: [app.env]}
  cgrules: {image: a, device_cgroup_rules: ["c 1:3 mr"]}
  cgroup: {image: a, cgroup: host}
  uts: {image: a, uts: host}
secrets:
  shadow: {file: /etc/shadow}
configs:
  keys: {file: /root/.ssh/authorized_keys}
`, nil)
	wantReasons(t, got, map[string][]string{
		"secret":  {HostAccessHostEtc},
		"config":  {HostAccessHostHome},
		"ctxabs":  {HostAccessBuildContext, HostAccessHostRoot},
		"ctxup":   {HostAccessBuildContext},
		"envabs":  {HostAccessEnvFile, HostAccessHostProc},
		"envup":   {HostAccessEnvFile},
		"cgrules": {HostAccessDevices},
		"cgroup":  {HostAccessCgroupHost},
		"uts":     {HostAccessUTSHost},
	})
	for _, svc := range []string{"ctxok", "envok"} {
		if len(got[svc]) != 0 {
			t.Errorf("%s stays inside the project: %v", svc, got[svc])
		}
	}
}

// TestSecurityOptOverrides is M3: any seccomp profile but the built-in one, any AppArmor
// profile but docker-default, and an SELinux type or disable.
func TestSecurityOptOverrides(t *testing.T) {
	for _, opt := range []string{
		`seccomp={"defaultAction":"SCMP_ACT_ALLOW"}`, "seccomp=/etc/permissive.json", "seccomp:unconfined",
		"apparmor=my-profile", "apparmor:unconfined", "label=type:container_runtime_t", "label:disable", "systempaths=unconfined",
	} {
		if !SecurityOptUnconfined([]string{opt}) {
			t.Errorf("%s is not reported", opt)
		}
	}
	for _, opt := range []string{"no-new-privileges", "no-new-privileges:true", "seccomp=builtin", "apparmor=docker-default", "label=level:s0:c100,c200", "label=user:system_u"} {
		if SecurityOptUnconfined([]string{opt}) {
			t.Errorf("%s is reported but keeps the default confinement", opt)
		}
	}
}
