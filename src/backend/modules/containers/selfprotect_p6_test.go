// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/http"
	"strings"
	"testing"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

const (
	spJoinID = "121212121212000000000000000000000000000000000000000000000000000a"
	spVolID  = "343434343434000000000000000000000000000000000000000000000000000b"
)

// p6Daemon is McHarbor's own daemon plus a container sharing McHarbor's PID namespace, one
// mounting a volume created with local bind options onto /, and that volume.
func p6Daemon() *spDaemon {
	return &spDaemon{
		docs: map[string]string{
			"harbor": spDocs["harbor"],
			"web":    spDocs["web"],
			"joined": `{"Id":"` + spJoinID + `","Name":"/joined","State":{"Status":"running","Running":true},` +
				`"Config":{"Image":"alpine","Env":[],"Labels":{}},"HostConfig":{"PidMode":"container:` + spSelfID + `"},"NetworkSettings":{"Networks":{}}}`,
			"voluser": `{"Id":"` + spVolID + `","Name":"/voluser","State":{"Status":"running","Running":true},` +
				`"Config":{"Image":"alpine","Env":[],"Labels":{}},"HostConfig":{"Binds":["sneaky:/h"]},"NetworkSettings":{"Networks":{}}}`,
		},
		vols: map[string]string{
			"sneaky": `{"Name":"sneaky","Driver":"local","Mountpoint":"/var/lib/docker/volumes/sneaky/_data","Options":{"type":"none","o":"bind","device":"/"},"Scope":"local"}`,
		},
	}
}

func assertRefused(t *testing.T, f *spFixture, d *spDaemon, name, target, body, reason, call string) {
	t.Helper()
	code, resp := f.do(t, "ops", http.MethodPost, target, body)
	if code != http.StatusForbidden || !strings.Contains(resp, "err.host_self.host_access_required") || !strings.Contains(resp, `"`+reason+`"`) {
		t.Errorf("%s by ops: HTTP %d %s; want 403 naming %s", name, code, resp, reason)
	}
	if call != "" && d.called(call) {
		t.Errorf("%s: the refused request reached Docker", name)
	}
}

// TestNamespaceJoinOnTheSelfEnvironmentNeedsHostSelf is C1: creating a container in another
// container's pid, ipc or network namespace, and a shell in one that already is.
func TestNamespaceJoinOnTheSelfEnvironmentNeedsHostSelf(t *testing.T) {
	d := p6Daemon()
	f := newSPFixtureWith(t, map[string]*spDaemon{"envC": d})
	for mode, hc := range map[string]string{
		"pid": `{"PidMode":"container:harbor"}`,
		"ipc": `{"IpcMode":"container:harbor"}`,
		"net": `{"NetworkMode":"container:harbor"}`,
	} {
		req := `{"name":"join` + mode + `","image":"alpine","hostConfig":` + hc + `}`
		assertRefused(t, f, d, mode+" join", "/containers?env=envC", req, coredocker.HostAccessJoinsNamespace, "create:join"+mode+":")
		if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envC", req); code != http.StatusCreated {
			t.Errorf("%s join by a host_self.manage holder: HTTP %d %s; want 201", mode, code, body)
		}
	}
	assertRefused(t, f, d, "exec in a container sharing McHarbor's PID namespace", "/containers/joined/exec?env=envC", `{"command":["id"]}`, coredocker.HostAccessJoinsNamespace, "exec:joined")
	if code, body := f.do(t, "ops", http.MethodPost, "/containers/web/exec?env=envC", `{"command":["id"]}`); strings.Contains(body, "err.host_self") {
		t.Errorf("exec in an ordinary container: HTTP %d %s", code, body)
	}
}

// TestVolumeDisguisedBindOnTheSelfEnvironmentNeedsHostSelf is C2 for containers: a Mounts
// volume carrying local bind options, and an existing volume created with them, both for a
// new container and for a shell in one that mounts it.
func TestVolumeDisguisedBindOnTheSelfEnvironmentNeedsHostSelf(t *testing.T) {
	d := p6Daemon()
	f := newSPFixtureWith(t, map[string]*spDaemon{"envC": d})
	mounts := `{"name":"mnt","image":"alpine","hostConfig":{"Mounts":[{"Type":"volume","Source":"fresh","Target":"/h",` +
		`"VolumeOptions":{"DriverConfig":{"Name":"local","Options":{"type":"none","o":"bind","device":"/"}}}}]}}`
	assertRefused(t, f, d, "Mounts volume with bind options", "/containers?env=envC", mounts, coredocker.HostAccessHostRoot, "create:mnt:")
	existing := `{"name":"reuse","image":"alpine","hostConfig":{"Binds":["sneaky:/h"]}}`
	assertRefused(t, f, d, "existing bind-option volume", "/containers?env=envC", existing, coredocker.HostAccessHostRoot, "create:reuse:")
	if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envC", existing); code != http.StatusCreated {
		t.Errorf("existing bind-option volume by a host_self.manage holder: HTTP %d %s; want 201", code, body)
	}
	assertRefused(t, f, d, "exec in a container mounting the bind-option volume", "/containers/voluser/exec?env=envC", `{"command":["id"]}`, coredocker.HostAccessHostRoot, "exec:voluser")
	plain := `{"name":"plainvol","image":"alpine","hostConfig":{"Binds":["unknownvol:/data"]}}`
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envC", plain); code != http.StatusCreated {
		t.Errorf("a fresh named volume by ops: HTTP %d %s; want 201", code, body)
	}
}

// TestHostBindDenylistOnTheSelfEnvironment is H1: a data bind (an operator's /srv tree, an App
// Store app-data path) needs no grant, while a parent of a denied path such as /var does.
func TestHostBindDenylistOnTheSelfEnvironment(t *testing.T) {
	f := newSPFixture(t)
	for name, bind := range map[string]string{"media": "/srv/media/films:/films:ro", "jelly": "/opt/appdata/jellyfin/config:/config"} {
		req := `{"name":"` + name + `","image":"jellyfin","hostConfig":{"Binds":["` + bind + `"]}}`
		if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", req); code != http.StatusCreated {
			t.Errorf("%s by ops: HTTP %d %s; want 201", bind, code, body)
		}
	}
	for name, tc := range map[string][2]string{
		"var":  {"/var:/v", coredocker.HostAccessDockerData},
		"home": {"/home/../home/op/.ssh:/k", coredocker.HostAccessHostHome},
		"lib":  {"/usr/lib/:/l", coredocker.HostAccessHostSystem},
	} {
		req := `{"name":"deny` + name + `","image":"alpine","hostConfig":{"Binds":["` + tc[0] + `"]}}`
		assertRefused(t, f, f.self, tc[0], "/containers?env=envA", req, tc[1], "create:deny"+name+":")
	}
}

// TestSecurityOptOverridesOnTheSelfEnvironment is M3: a custom seccomp profile, an AppArmor
// override and an SELinux type each need host_self.manage there.
func TestSecurityOptOverridesOnTheSelfEnvironment(t *testing.T) {
	f := newSPFixture(t)
	for name, opt := range map[string]string{
		"seccomp":  `seccomp={\"defaultAction\":\"SCMP_ACT_ALLOW\"}`,
		"apparmor": `apparmor=permissive-profile`,
		"label":    `label=type:container_runtime_t`,
	} {
		req := `{"name":"so` + name + `","image":"alpine","hostConfig":{"SecurityOpt":["` + opt + `"]}}`
		assertRefused(t, f, f.self, name, "/containers?env=envA", req, coredocker.HostAccessSecurityOpt, "create:so"+name+":")
	}
}
