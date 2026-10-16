// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/http"
	"slices"
	"testing"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

// The TOFIX-03 scenario: /tmp/x is an ordinary bind, a container planted s -> docker.sock in
// it, and a second container now binds /tmp/x/s. The path string passes the denylist.
const symlinkCreate = `{"name":"planted","image":"docker:cli","hostConfig":{"Binds":["/tmp/x/s:/var/run/docker.sock","/srv/site:/site"]}}`

// TestSymlinkedBindSourceNeedsHostSelf is TOFIX-03 end to end through the fake daemon: the
// helper reports the link, ops is refused with the reason and an audit row, and root, holding
// host_self.manage, creates the container.
func TestSymlinkedBindSourceNeedsHostSelf(t *testing.T) {
	f := newSPFixture(t)
	f.self.links = map[string]bool{"/tmp/x/s": true}

	assertRefused(t, f, f.self, "symlinked bind source", "/containers?env=envA", symlinkCreate, coredocker.HostAccessBindSymlink, "create:planted:")
	if rows := f.deniedRows(t); len(rows) == 0 {
		t.Error("the refusal left no host_self.denied audit row")
	}
	if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envA", symlinkCreate); code != http.StatusCreated || !f.self.called("create:planted:") {
		t.Fatalf("symlinked bind by root (host_self.manage): HTTP %d %s; want 201", code, body)
	}
}

// TestPlainBindSourceRunsOneLockedDownHelper checks a create with several plain host binds
// goes ahead after exactly one helper, and that helper is the one the design promises: no
// network, read-only root, only CAP_DAC_READ_SEARCH, the host's / bound read-only, labelled.
func TestPlainBindSourceRunsOneLockedDownHelper(t *testing.T) {
	f := newSPFixture(t)
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", symlinkCreate); code != http.StatusCreated {
		t.Fatalf("plain host binds by ops: HTTP %d %s; want 201", code, body)
	}
	runs := f.self.helpers()
	if len(runs) != 1 {
		t.Fatalf("%d helpers ran for one create, want 1", len(runs))
	}
	h := runs[0]
	if !slices.Equal(h.Cmd, []string{"/tmp/x/s", "/srv/site"}) {
		t.Errorf("helper checked %v, want both sources in one run", h.Cmd)
	}
	hc := h.HostConfig
	if hc.NetworkMode != "none" || !hc.ReadonlyRootfs || !slices.Equal(hc.CapDrop, []string{"ALL"}) || !slices.Equal(hc.CapAdd, []string{"CAP_DAC_READ_SEARCH"}) || !slices.Contains(hc.SecurityOpt, "no-new-privileges") {
		t.Errorf("helper host config is not locked down: %+v", hc)
	}
	if len(hc.Mounts) != 1 || hc.Mounts[0].Source != "/" || !hc.Mounts[0].ReadOnly || hc.Mounts[0].Type != "bind" {
		t.Errorf("helper mounts %+v, want only / read-only", hc.Mounts)
	}
	if !f.self.called("delete:" + spHelperID) {
		t.Error("the helper was not removed")
	}

	// Not McHarbor's environment: no helper at all.
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envB", symlinkCreate); code != http.StatusCreated {
		t.Fatalf("binds on another environment: HTTP %d %s; want 201", code, body)
	}
	if n := len(f.other.helpers()); n != 0 {
		t.Errorf("%d helpers ran on an environment that is not McHarbor's", n)
	}
}

// TestAlreadyRefusedCreateRunsNoHelper keeps the helper off a create the denylist refuses anyway.
func TestAlreadyRefusedCreateRunsNoHelper(t *testing.T) {
	f := newSPFixture(t)
	req := `{"name":"sock","image":"docker:cli","hostConfig":{"Binds":["/var/run/docker.sock:/var/run/docker.sock","/srv/site:/site"]}}`
	assertRefused(t, f, f.self, "socket bind", "/containers?env=envA", req, coredocker.HostAccessDockerSocket, "create:sock:")
	if n := len(f.self.helpers()); n != 0 {
		t.Errorf("%d helpers ran for a create already refused", n)
	}
}

// TestBindCheckHelperFailureFailsClosed covers the helper exiting non-zero and the daemon
// holding no image to start it from (it never pulls): both refuse as unresolved_source.
func TestBindCheckHelperFailureFailsClosed(t *testing.T) {
	f := newSPFixture(t)
	f.self.helperExit = 1
	req := `{"name":"broken","image":"nginx","hostConfig":{"Binds":["/srv/site:/site"]}}`
	assertRefused(t, f, f.self, "helper exit 1", "/containers?env=envA", req, coredocker.HostAccessUnresolved, "create:broken:")

	g := newSPFixture(t)
	g.self.noImages = true
	assertRefused(t, g, g.self, "no local image", "/containers?env=envA", req, coredocker.HostAccessUnresolved, "create:broken:")
	if n := len(g.self.helpers()); n != 0 {
		t.Errorf("a helper was created with no local image: %d", n)
	}
}

// TestVolumeDisguisedSymlinkBindNeedsHostSelf is the same link reached through a volume mount
// whose local options bind it.
func TestVolumeDisguisedSymlinkBindNeedsHostSelf(t *testing.T) {
	f := newSPFixture(t)
	f.self.links = map[string]bool{"/tmp/x": true}
	req := `{"name":"vol","image":"alpine","hostConfig":{"Mounts":[{"Type":"volume","Source":"fresh","Target":"/s",` +
		`"VolumeOptions":{"DriverConfig":{"Name":"local","Options":{"type":"none","o":"bind","device":"/tmp/x/s"}}}}]}}`
	assertRefused(t, f, f.self, "volume-disguised symlinked bind", "/containers?env=envA", req, coredocker.HostAccessBindSymlink, "create:vol:")
}
