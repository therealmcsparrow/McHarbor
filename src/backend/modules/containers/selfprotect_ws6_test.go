// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/http"
	"strings"
	"testing"
)

// TestCreateTimeRulesOnTheSelfEnvironmentNeedHostSelf covers Q20: devices, an unconfined
// security option, the host's user, IPC or network namespace, and any added capability
// each need host_self.manage to create on the environment McHarbor runs on, and nothing
// extra anywhere else.
func TestCreateTimeRulesOnTheSelfEnvironmentNeedHostSelf(t *testing.T) {
	cases := []struct{ name, hostConfig, reason string }{
		{"devices", `{"Devices":[{"PathOnHost":"/dev/sda","PathInContainer":"/dev/sda","CgroupPermissions":"rwm"}]}`, "devices"},
		{"apparmor", `{"SecurityOpt":["apparmor=unconfined"]}`, "security_opt"},
		{"seccomp", `{"SecurityOpt":["seccomp:unconfined"]}`, "security_opt"},
		{"userns", `{"UsernsMode":"host"}`, "userns_host"},
		{"ipc", `{"IpcMode":"host"}`, "ipc_host"},
		{"netadmin", `{"CapAdd":["NET_ADMIN"]}`, "cap_add"},
		{"hostnet", `{"NetworkMode":"host"}`, "network_host"},
	}
	f := newSPFixture(t)
	for _, tc := range cases {
		req := `{"name":"` + tc.name + `","image":"alpine","hostConfig":` + tc.hostConfig + `}`
		code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", req)
		if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.host_access_required") || !strings.Contains(body, `"`+tc.reason+`"`) {
			t.Errorf("%s on the self environment by ops: HTTP %d %s; want 403 naming %s", tc.name, code, body, tc.reason)
		}
		if f.self.called("create:" + tc.name + ":") {
			t.Errorf("%s: the refused create reached Docker", tc.name)
		}
		if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envA", req); code != http.StatusCreated {
			t.Errorf("%s on the self environment by a host_self.manage holder: HTTP %d %s; want 201", tc.name, code, body)
		}
		if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envB", req); code != http.StatusCreated {
			t.Errorf("%s on another environment by ops: HTTP %d %s; want 201", tc.name, code, body)
		}
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", `{"name":"confined","image":"alpine","hostConfig":{"SecurityOpt":["no-new-privileges"]}}`); code != http.StatusCreated {
		t.Errorf("a tightening security option on the self environment: HTTP %d %s; want 201", code, body)
	}
}

const (
	spPeerID = "aaaaaaaaaaaa0000000000000000000000000000000000000000000000000007"
	spPGID   = "bbbbbbbbbbbb0000000000000000000000000000000000000000000000000008"
)

var (
	spPeerDoc = `{"Id":"` + spPeerID + `","Name":"/prod-mcharbor-node-2-1","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"registry.local/ops/harbor:9","Env":[],"Labels":{"com.docker.compose.service":"mcharbor-node-2"}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}},"Mounts":[` +
		`{"Type":"volume","Name":"prod_node2_data","Source":"/var/lib/docker/volumes/prod_node2_data/_data","Destination":"/app/data","RW":true}]}`
	spPGDoc = `{"Id":"` + spPGID + `","Name":"/prod-postgres-1","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"postgres:17-alpine","Env":[],"Labels":{"com.mcharbor.protected":"true"}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}},"Mounts":[` +
		`{"Type":"volume","Name":"prod_mcharbor-postgres","Source":"/var/lib/docker/volumes/prod_mcharbor-postgres/_data","Destination":"/var/lib/postgresql/data","RW":true}]}`
)

// TestPeerNodeDaemonIsASelfEnvironment covers T6.7 and T6.8: a daemon running another
// McHarbor node is McHarbor's host for the create-time checks, the HA PostgreSQL's volume
// there is off limits like McHarbor's own, and its protected label gates its lifecycle.
// A daemon with the same PostgreSQL but no McHarbor node stays ordinary.
func TestPeerNodeDaemonIsASelfEnvironment(t *testing.T) {
	peer := &spDaemon{docs: map[string]string{"peer": spPeerDoc, "pg": spPGDoc, "web": spDocs["web"]}}
	lone := &spDaemon{docs: map[string]string{"pg": spPGDoc, "web": spDocs["web"]}}
	f := newSPFixtureWith(t, map[string]*spDaemon{"envC": peer, "envD": lone})

	sock := `{"name":"probe","image":"docker:cli","hostConfig":{"Binds":["/var/run/docker.sock:/var/run/docker.sock"]}}`
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envC", sock); code != http.StatusForbidden || !strings.Contains(body, "docker_socket") {
		t.Errorf("socket container beside a peer node by ops: HTTP %d %s; want 403 naming docker_socket", code, body)
	}
	if peer.called("create:probe:") {
		t.Error("the refused create on the peer's daemon reached Docker")
	}
	pgVol := `{"name":"reader","image":"alpine","hostConfig":{"Binds":["prod_mcharbor-postgres:/data:ro"]}}`
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envC", pgVol); code != http.StatusForbidden || !strings.Contains(body, "mcharbor_volume") {
		t.Errorf("mounting the HA PostgreSQL volume beside a peer by ops: HTTP %d %s; want 403 naming mcharbor_volume", code, body)
	}
	peerVol := `{"name":"peerdata","image":"alpine","hostConfig":{"Binds":["prod_node2_data:/data"]}}`
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envC", peerVol); code != http.StatusForbidden || !strings.Contains(body, "mcharbor_volume") {
		t.Errorf("mounting the peer's data volume by ops: HTTP %d %s; want 403 naming mcharbor_volume", code, body)
	}
	if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envC", sock); code != http.StatusCreated {
		t.Errorf("socket container beside a peer by a host_self.manage holder: HTTP %d %s; want 201", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envD", sock); code != http.StatusCreated {
		t.Errorf("socket container on a daemon with no McHarbor node: HTTP %d %s; want 201", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers/pg/stop?env=envD", ""); code != http.StatusForbidden || !strings.Contains(body, `"target":"protected"`) {
		t.Errorf("stopping the protected PostgreSQL by ops: HTTP %d %s; want 403 target protected", code, body)
	}
	if lone.called("stop:pg") {
		t.Error("the refused stop reached Docker")
	}
}
