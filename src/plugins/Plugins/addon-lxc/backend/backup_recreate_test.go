// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// fullManifest is a manifest exactly as backupManifestFor writes one.
const fullManifest = `{
  "version": 1,
  "containerId": "ct0001",
  "name": "web",
  "description": "front door",
  "environmentId": "env-a",
  "osFamily": "debian",
  "osVersion": "12",
  "initSystem": "openrc",
  "privileged": true,
  "nesting": true,
  "poolBackend": "zfs",
  "rootfsPath": "/tank/mcharbor/containers/ct0001/rootfs",
  "limits": {"cpuMax": "200000 100000", "memoryMaxBytes": 2147483648, "swapMaxBytes": 0, "pidsMax": 4096},
  "nics": [
    {"id": "n2", "slot": 1, "networkId": "net-b", "bridge": "br1", "guestName": "eth1", "mac": "aa", "addressMode": "static", "staticIp": "10.0.0.5/24", "staticGw": "10.0.0.1"},
    {"id": "n1", "slot": 0, "networkId": "net-a", "bridge": "br0", "guestName": "eth0", "mac": "bb", "addressMode": "dhcp"}
  ],
  "mounts": [
    {"id": "m1", "kind": "volume", "source": "data", "destination": "/srv", "readonly": true, "shiftIds": true}
  ],
  "takenAt": "2026-09-26T02:00:00Z"
}`

func TestParseBackupManifest(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"a manifest as written", fullManifest, false},
		{"minimal but versioned", `{"version": 1, "name": "x"}`, false},
		{"empty object", ``, true},
		{"whitespace only", "  \n ", true},
		{"not json", `this is a tarball`, true},
		{"truncated json", `{"version": 1, "name": "web"`, true},
		{"limits of the wrong type", `{"version": 1, "limits": "big"}`, true},
		{"no version", `{"name": "web"}`, true},
		{"a newer version", `{"version": 2, "name": "web"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseBackupManifest([]byte(tc.raw))
			if tc.wantErr {
				if !errors.Is(err, ErrManifestUnreadable) {
					t.Fatalf("err = %v, want ErrManifestUnreadable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func mustManifest(t *testing.T, raw string) backupManifest {
	t.Helper()
	manifest, err := parseBackupManifest([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return manifest
}

func TestDraftFromManifestCarriesTheDefinition(t *testing.T) {
	draft := draftFromManifest(mustManifest(t, fullManifest))
	create := draft.Create

	if len(draft.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", draft.Warnings)
	}
	if draft.SourceContainerID != "ct0001" || draft.TakenAt != "2026-09-26T02:00:00Z" {
		t.Errorf("source = %q taken %q", draft.SourceContainerID, draft.TakenAt)
	}
	if draft.OSFamily != "debian" || draft.OSVersion != "12" {
		t.Errorf("os = %q %q", draft.OSFamily, draft.OSVersion)
	}
	if create.Name != "web" || create.Description != "front door" || create.EnvironmentID != "env-a" {
		t.Errorf("identity = %q %q %q", create.Name, create.Description, create.EnvironmentID)
	}
	if create.InitSystem != InitOpenRC || !create.Privileged || !create.Nesting {
		t.Errorf("profile = %q privileged=%v nesting=%v", create.InitSystem, create.Privileged, create.Nesting)
	}
	if create.PoolBackend != "zfs" || create.PoolTarget != "/tank/mcharbor" || create.PoolID != "" {
		t.Errorf("pool = %q %q %q", create.PoolBackend, create.PoolTarget, create.PoolID)
	}
	if create.ImageID != "" || create.TemplateID != "" {
		t.Errorf("a recreate draft must not name an image or template")
	}
	want := Limits{CPUMax: "200000 100000", MemoryMaxBytes: 2147483648, PidsMax: 4096}
	if !reflect.DeepEqual(create.Limits, want) {
		t.Errorf("limits = %+v, want %+v", create.Limits, want)
	}
	if len(create.NICs) != 2 || create.NICs[0].Bridge != "br0" || create.NICs[1].Bridge != "br1" {
		t.Fatalf("nics out of slot order: %+v", create.NICs)
	}
	if create.NICs[1].AddressMode != "static" || create.NICs[1].StaticIP != "10.0.0.5/24" ||
		create.NICs[1].NetworkID != "net-b" || create.NICs[1].GuestName != "eth1" {
		t.Errorf("static nic = %+v", create.NICs[1])
	}
	if len(create.Mounts) != 1 {
		t.Fatalf("mounts = %+v", create.Mounts)
	}
	mount := create.Mounts[0]
	if mount.Kind != MountVolume || mount.Source != "data" || mount.Destination != "/srv" ||
		!mount.ReadOnly || mount.ShiftIDs == nil || !*mount.ShiftIDs {
		t.Errorf("mount = %+v", mount)
	}
	if err := validateRecreate(func() CreateRequest { c := create; c.PoolID = "pool-1"; return c }()); err != nil {
		t.Errorf("the draft of a well-formed manifest should validate once a pool is picked: %v", err)
	}
}

func TestDraftFromManifestRepairsWhatItCan(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		warnings []string
		check    func(t *testing.T, create CreateRequest)
	}{
		{
			name:     "missing optional fields",
			raw:      `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "systemd", "rootfsPath": "/p/containers/c1/rootfs"}`,
			warnings: []string{},
			check: func(t *testing.T, create CreateRequest) {
				if create.NICs == nil || create.Mounts == nil || len(create.NICs)+len(create.Mounts) != 0 {
					t.Errorf("nics/mounts should be empty, non-nil slices: %v %v", create.NICs, create.Mounts)
				}
				if create.PoolTarget != "/p" {
					t.Errorf("pool target = %q", create.PoolTarget)
				}
			},
		},
		{
			name:     "missing init system",
			raw:      `{"version": 1, "containerId": "c1", "name": "db", "rootfsPath": "/p/containers/c1/rootfs"}`,
			warnings: []string{warnInitUnknown},
			check: func(t *testing.T, create CreateRequest) {
				if create.InitSystem != InitSystemd {
					t.Errorf("init = %q, want systemd", create.InitSystem)
				}
			},
		},
		{
			name:     "an init this plugin does not run",
			raw:      `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "runit", "rootfsPath": "/p/containers/c1/rootfs"}`,
			warnings: []string{warnInitUnknown},
		},
		{
			name:     "missing name",
			raw:      `{"version": 1, "containerId": "c1", "initSystem": "none", "rootfsPath": "/p/containers/c1/rootfs"}`,
			warnings: []string{warnNameInvalid},
		},
		{
			name:     "a name the create route refuses",
			raw:      `{"version": 1, "containerId": "c1", "name": "Web_01", "initSystem": "none", "rootfsPath": "/p/containers/c1/rootfs"}`,
			warnings: []string{warnNameInvalid},
		},
		{
			name:     "a rootfs outside the plugin's layout",
			raw:      `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "none", "rootfsPath": "/somewhere/else"}`,
			warnings: []string{warnPoolTargetUnknown},
			check: func(t *testing.T, create CreateRequest) {
				if create.PoolTarget != "" {
					t.Errorf("pool target = %q, want empty", create.PoolTarget)
				}
			},
		},
		{
			name: "an interface with nothing to attach to",
			raw: `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "none", "rootfsPath": "/p/containers/c1/rootfs",
			       "nics": [{"slot": 0, "networkId": "n"}, {"slot": 1, "attachKind": "macvlan", "parentLink": "enp1s0"}]}`,
			warnings: []string{warnNICDropped},
			check: func(t *testing.T, create CreateRequest) {
				if len(create.NICs) != 1 || create.NICs[0].ParentLink != "enp1s0" {
					t.Errorf("nics = %+v", create.NICs)
				}
			},
		},
		{
			name: "mounts with an unknown kind or no endpoints",
			raw: `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "none", "rootfsPath": "/p/containers/c1/rootfs",
			       "mounts": [{"kind": "nfs", "source": "a", "destination": "/a"}, {"kind": "bind", "source": "", "destination": "/b"},
			                  {"kind": "bind", "source": "/host", "destination": "/c", "shiftIds": false}]}`,
			warnings: []string{warnMountDropped},
			check: func(t *testing.T, create CreateRequest) {
				if len(create.Mounts) != 1 || create.Mounts[0].Source != "/host" ||
					create.Mounts[0].ShiftIDs == nil || *create.Mounts[0].ShiftIDs {
					t.Errorf("mounts = %+v", create.Mounts)
				}
			},
		},
		{
			name: "negative limits",
			raw: `{"version": 1, "containerId": "c1", "name": "db", "initSystem": "none", "rootfsPath": "/p/containers/c1/rootfs",
			       "limits": {"memoryMaxBytes": -1, "pidsMax": 100, "swapMaxBytes": 0}}`,
			warnings: []string{warnLimitsAdjusted},
			check: func(t *testing.T, create CreateRequest) {
				if create.Limits.MemoryMaxBytes != 0 || create.Limits.PidsMax != 100 {
					t.Errorf("limits = %+v", create.Limits)
				}
				if err := validateLimits(create.Limits); err != nil {
					t.Errorf("clamped limits still refused: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draft := draftFromManifest(mustManifest(t, tc.raw))
			if !reflect.DeepEqual(draft.Warnings, tc.warnings) {
				t.Errorf("warnings = %v, want %v", draft.Warnings, tc.warnings)
			}
			if tc.check != nil {
				tc.check(t, draft.Create)
			}
		})
	}
}

func TestPoolTargetFromRootfs(t *testing.T) {
	cases := []struct{ rootfs, id, want string }{
		{"/tank/mcharbor/containers/abc/rootfs", "abc", "/tank/mcharbor"},
		{"/tank/mcharbor/containers/abc/rootfs/", "abc", "/tank/mcharbor"},
		{"/containers/abc/rootfs", "abc", "/"},
		{"/tank/containers/other/rootfs", "abc", ""},
		{"/tank/abc/rootfs", "abc", ""},
		{"", "abc", ""},
		{"/tank/containers/abc/rootfs", "", ""},
	}
	for _, tc := range cases {
		if got := poolTargetFromRootfs(tc.rootfs, tc.id); got != tc.want {
			t.Errorf("poolTargetFromRootfs(%q, %q) = %q, want %q", tc.rootfs, tc.id, got, tc.want)
		}
	}
}

func TestApplyRecreateOverrides(t *testing.T) {
	fromManifest := CreateRequest{EnvironmentID: "env-a", Name: "web", PoolBackend: "zfs", PoolTarget: "/tank"}

	got := applyRecreateOverrides(fromManifest, RecreateRequest{
		Name: " web-restored ", PoolID: "pool-9", StartAfterRestore: true,
	})
	if got.Name != "web-restored" || got.PoolID != "pool-9" || got.EnvironmentID != "env-a" ||
		got.PoolTarget != "/tank" || !got.StartAfterCreate {
		t.Errorf("flat overrides on the manifest = %+v", got)
	}

	edited := CreateRequest{EnvironmentID: "env-b", Name: "edited", PoolID: "p", PoolTarget: "/x"}
	got = applyRecreateOverrides(fromManifest, RecreateRequest{Create: &edited, PoolTarget: "/y"})
	if got.Name != "edited" || got.EnvironmentID != "env-b" || got.PoolTarget != "/y" || got.PoolBackend != "" {
		t.Errorf("a full body replaces the manifest wholesale: %+v", got)
	}

	got = applyRecreateOverrides(fromManifest, RecreateRequest{})
	if !reflect.DeepEqual(got, fromManifest) {
		t.Errorf("no overrides changed the manifest: %+v", got)
	}
}

func TestValidateRecreateRefusesAnImageOrTemplate(t *testing.T) {
	base := CreateRequest{EnvironmentID: "e", Name: "web", PoolID: "p", PoolTarget: "/t"}
	if err := validateRecreate(base); err != nil {
		t.Fatalf("a plain definition should validate: %v", err)
	}
	for _, mutate := range []func(*CreateRequest){
		func(c *CreateRequest) { c.ImageID = "img" },
		func(c *CreateRequest) { c.TemplateID = "tpl" },
		func(c *CreateRequest) { c.PoolID = "" },
		func(c *CreateRequest) { c.Name = "" },
	} {
		req := base
		mutate(&req)
		if validateRecreate(req) == nil {
			t.Errorf("validateRecreate accepted %+v", req)
		}
	}
	if err := validateCreate(base); err == nil {
		t.Errorf("an ordinary create still needs an image or a template")
	}
}

// grantSet is an RBAC handle that grants exactly the listed keys.
type grantSet struct {
	keys map[string]bool
	err  error
}

func (g grantSet) HasPermission(_, _, perm string) (bool, error) {
	if g.err != nil {
		return false, g.err
	}
	return g.keys[perm], nil
}

type silentLogger struct{}

func (silentLogger) Info(string, ...any)  {}
func (silentLogger) Warn(string, ...any)  {}
func (silentLogger) Error(string, ...any) {}
func (silentLogger) Debug(string, ...any) {}

func TestAllowRecreateNeedsBothKeys(t *testing.T) {
	plain := CreateRequest{EnvironmentID: "env-a", Name: "web"}
	privileged := CreateRequest{EnvironmentID: "env-a", Name: "web", Privileged: true}
	withBind := CreateRequest{EnvironmentID: "env-a", Name: "web",
		Mounts: []CreateMountRequest{{Kind: MountBind, Source: "/h", Destination: "/c"}}}

	cases := []struct {
		name     string
		rbac     permissionChecker
		create   CreateRequest
		allowed  bool
		wantCode string
	}{
		{"both keys", grantSet{keys: map[string]bool{"ct.restore": true, "ct.manage": true}}, plain, true, ""},
		{"restore only", grantSet{keys: map[string]bool{"ct.restore": true}}, plain, false, codeRecreateNeedsManage},
		{"manage only", grantSet{keys: map[string]bool{"ct.manage": true}}, plain, false, codeRecreateNeedsRestore},
		{"neither", grantSet{keys: map[string]bool{}}, plain, false, codeRecreateNeedsRestore},
		{"lookup fails", grantSet{err: errors.New("rbac down")}, plain, false, codeRecreateNeedsRestore},
		{"no rbac handle", nil, plain, false, codeRecreateNeedsRestore},
		{"privileged without ct.privileged",
			grantSet{keys: map[string]bool{"ct.restore": true, "ct.manage": true}}, privileged, false, codeForbidden},
		{"privileged with ct.privileged",
			grantSet{keys: map[string]bool{"ct.restore": true, "ct.manage": true, "ct.privileged": true}}, privileged, true, ""},
		{"host bind without ct.privileged",
			grantSet{keys: map[string]bool{"ct.restore": true, "ct.manage": true}}, withBind, false, codeForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{rbac: tc.rbac, logger: silentLogger{}}
			if tc.rbac == nil {
				h.rbac = nil
			}
			rec := httptest.NewRecorder()
			got := h.allowRecreate(rec, httptest.NewRequest(http.MethodPost, "/", nil), "user-1", tc.create)
			if got != tc.allowed {
				t.Fatalf("allowRecreate = %v, want %v", got, tc.allowed)
			}
			if tc.allowed {
				if rec.Body.Len() != 0 {
					t.Errorf("an allowed recreate wrote a response: %s", rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			var envelope apiEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("body: %v", err)
			}
			if envelope.Success || envelope.Code != tc.wantCode {
				t.Errorf("envelope = %+v, want code %s", envelope, tc.wantCode)
			}
		})
	}
}

// TestRecreateRoutesAreGated reads plugin.yaml the way the host does: the
// recreate route must carry ct.restore through the middleware, and the two reads
// ct.backup, or the host mounts them with no permission check at all.
func TestRecreateRoutesAreGated(t *testing.T) {
	routes := readManifestRoutes(t)
	want := map[string]string{
		"POST /api/system-containers/backups/{runId}/recreate": "ct.restore",
		"GET /api/system-containers/backups/{runId}/manifest":  "ct.backup",
		"GET /api/system-containers/orphaned-backups":          "ct.backup",
	}
	for key, perm := range want {
		route, found := routes[key]
		if !found {
			t.Errorf("%s is not declared in plugin.yaml", key)
			continue
		}
		if route.public || route.requires != perm {
			t.Errorf("%s: public=%v requires=%q, want %q", key, route.public, route.requires, perm)
		}
	}
	if !strings.Contains(codeRecreateNeedsManage, "manage") {
		t.Errorf("the manage refusal code should name the key")
	}
}
