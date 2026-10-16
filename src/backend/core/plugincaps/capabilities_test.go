// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const baseManifest = `
slug: addon-demo
adds_permissions:
  - key: "demo.view"
  - key: "demo.backup.manage"
  - key: "demo.backup.delete"
  - key: "demo.restore.run"
consumes_permissions:
  - "compute.view"
contributes:
  routes:
    - method: GET
      path: "/api/demo"
    - method: DELETE
      path: "/api/demo/backups/{runId}"
    - method: DELETE
      path: "/api/demo/backup-plans/{planId}"
    - method: POST
      path: "/api/demo/auth"
      public: true
    - method: POST
      path: "/api/demo/private"
    - method: POST
      path: "/api/demo/reports"
    - method: GET
      path: "/api/demo/reports/{id}/export"
  capabilities:
`

func parseWith(t *testing.T, caps string) ([]Capability, error) {
	t.Helper()
	return Parse("addon-demo", []byte(baseManifest+caps))
}

func TestParseAcceptsEveryKind(t *testing.T) {
	caps, err := parseWith(t, `
    - kind: runbook-provider
      domain: vm
    - kind: backup-row-actions
      source_kind: vm
      pages: {open: "/plugins/addon-demo/{subject}", restore: "/plugins/addon-demo/{subject}/r/{id}", edit_plan: "/plugins/addon-demo/{subject}/p/{id}"}
      routes: {delete_run: "/api/demo/backups/{id}", delete_plan: "/api/demo/backup-plans/{id}"}
      permissions: {view: demo.view, manage: demo.backup.manage, delete: demo.backup.delete, restore: demo.restore.run}
    - kind: estate-source
      metric: vms
      path: "/api/demo"
      environment_field: environmentId
    - kind: lifecycle-subject
      subject_type: vm
      page: "/plugins/addon-demo"
    - kind: document-export
      formats: [pdf, docx]
      permission: compute.view
      generate_path: "/api/demo/reports"
      export_path: "/api/demo/reports/{id}/export"
    - kind: login-endpoint
      provider_type: ldap
      path: "/api/demo/auth"
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(caps) != 6 {
		t.Fatalf("want 6 capabilities, got %d", len(caps))
	}
	for _, c := range caps {
		if c.Plugin != "addon-demo" {
			t.Fatalf("capability %s not attributed to its plugin: %+v", c.Kind, c)
		}
	}
}

func TestParseWithoutCapabilitiesIsEmpty(t *testing.T) {
	caps, err := Parse("addon-demo", []byte("slug: addon-demo\ncontributes:\n  routes: []\n"))
	if err != nil || len(caps) != 0 {
		t.Fatalf("want no capabilities and no error, got %v %v", caps, err)
	}
}

func TestParseRefuses(t *testing.T) {
	cases := map[string]string{
		"unknown kind": `
    - kind: teleport
      domain: vm`,
		"missing required field": `
    - kind: estate-source
      metric: vms
      path: "/api/demo"`,
		"field from another kind": `
    - kind: runbook-provider
      domain: vm
      page: "/plugins/addon-demo"`,
		"bad token": `
    - kind: runbook-provider
      domain: "VM Stuff"`,
		"page under another plugin": `
    - kind: lifecycle-subject
      subject_type: vm
      page: "/plugins/addon-other"`,
		"page prefix trick": `
    - kind: lifecycle-subject
      subject_type: vm
      page: "/plugins/addon-demo-evil"`,
		"undeclared route": `
    - kind: estate-source
      metric: vms
      path: "/api/other"
      environment_field: environmentId`,
		"wrong method": `
    - kind: estate-source
      metric: vms
      path: "/api/demo/auth"
      environment_field: environmentId`,
		"login route not public": `
    - kind: login-endpoint
      provider_type: ldap
      path: "/api/demo/private"`,
		"foreign permission": `
    - kind: document-export
      formats: [pdf]
      permission: containers.manage
      generate_path: "/api/demo/reports"
      export_path: "/api/demo/reports/{id}/export"`,
		"backup map missing a key": `
    - kind: backup-row-actions
      source_kind: vm
      pages: {open: "/plugins/addon-demo", restore: "/plugins/addon-demo"}
      routes: {delete_run: "/api/demo/backups/{id}", delete_plan: "/api/demo/backup-plans/{id}"}
      permissions: {view: demo.view, manage: demo.backup.manage, delete: demo.backup.delete, restore: demo.restore.run}`,
		"backup map unknown key": `
    - kind: backup-row-actions
      source_kind: vm
      pages: {open: "/plugins/addon-demo", restore: "/plugins/addon-demo", edit_plan: "/plugins/addon-demo", extra: "/plugins/addon-demo"}
      routes: {delete_run: "/api/demo/backups/{id}", delete_plan: "/api/demo/backup-plans/{id}"}
      permissions: {view: demo.view, manage: demo.backup.manage, delete: demo.backup.delete, restore: demo.restore.run}`,
		"declared twice": `
    - kind: runbook-provider
      domain: vm
    - kind: runbook-provider
      domain: vm`,
	}
	for name, caps := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWith(t, caps); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestParseRefusesAnotherSlug(t *testing.T) {
	if _, err := Parse("addon-other", []byte(baseManifest)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a manifest for another slug must be refused, got %v", err)
	}
}

func TestRegistryResolvesOnlyLoadedPluginsDeterministically(t *testing.T) {
	r := NewRegistry()
	r.Set("addon-b", []Capability{{Kind: KindRunbookProvider, Domain: "vm"}})
	r.Set("addon-a", []Capability{{Kind: KindRunbookProvider, Domain: "vm"}, {Kind: KindLifecycleSubject, SubjectType: "vm", Page: "/plugins/addon-a"}})
	if c, ok := r.Provider(KindRunbookProvider, "vm"); !ok || c.Plugin != "addon-a" {
		t.Fatalf("two claimants must resolve to the first slug, got %+v %v", c, ok)
	}
	if got := len(r.OfKind(KindLifecycleSubject)); got != 1 {
		t.Fatalf("OfKind = %d, want 1", got)
	}
	r.Remove("addon-a")
	if c, ok := r.Provider(KindRunbookProvider, "vm"); !ok || c.Plugin != "addon-b" {
		t.Fatalf("after unload the remaining claimant answers, got %+v %v", c, ok)
	}
	r.Set("addon-b", nil)
	if _, ok := r.Provider(KindRunbookProvider, "vm"); ok || len(r.All()) != 0 {
		t.Fatal("an empty set must remove the plugin")
	}
}

// TestShippedManifestsDeclareValidCapabilities is the contract over the in-tree plugins:
// every plugin.yaml parses, and the facts the host's consumers rely on are declared by
// the plugin that owns them.
func TestShippedManifestsDeclareValidCapabilities(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(root, d.Name(), "plugin.yaml"))
		if err != nil {
			continue
		}
		caps, err := Parse(d.Name(), raw)
		if err != nil {
			t.Errorf("%s: %v", d.Name(), err)
			continue
		}
		reg.Set(d.Name(), caps)
	}
	want := []struct {
		kind Kind
		key  string
		slug string
	}{
		{KindRunbookProvider, "vm", "addon-kvm"},
		{KindRunbookProvider, "ct", "addon-lxc"},
		{KindRunbookProvider, "compute", "addon-compute"},
		{KindRunbookProvider, "swarm", "addon-swarm"},
		{KindRunbookProvider, "k8s", "addon-kubernetes"},
		{KindBackupRowActions, "vm", "addon-kvm"},
		{KindEstateSource, "vms", "addon-kvm"},
		{KindLifecycleSubject, "vm", "addon-kvm"},
		{KindLifecycleSubject, "pool", "addon-compute"},
		{KindLoginEndpoint, "ldap", "identity-ldap"},
		{KindPickerSource, "vm", "addon-kvm"},
		{KindWorkflowNodeAlias, "vm-trigger", "addon-kvm"},
		{KindEmailChannel, "smtp", "comm-smtp"},
		{KindEmailChannel, "gmail", "comm-gmail"},
		{KindEmailChannel, "exchange", "comm-exchange"},
		{KindConsoleSubject, "vm", "addon-kvm"},
		{KindConsoleSubject, "ct", "addon-lxc"},
		{KindRebootDrain, "vm", "addon-kvm"},
		{KindRebootDrain, "ct", "addon-lxc"},
	}
	for _, w := range want {
		c, ok := reg.Provider(w.kind, w.key)
		if !ok || c.Plugin != w.slug {
			t.Errorf("%s %q: want %s, got %q (%v)", w.kind, w.key, w.slug, c.Plugin, ok)
		}
	}
	// A workflow saved while the host shipped vm-trigger must keep resolving to the
	// node the KVM plugin now contributes, and its VM picker must list through vm.view.
	if action, ok := reg.NodeAliasAction("vm-trigger"); !ok || action != "plugin:addon-kvm:vm-trigger" {
		t.Errorf("stored vm-trigger: want plugin:addon-kvm:vm-trigger, got %q (%v)", action, ok)
	}
	// Workflows saved while the host shipped one send node per provider keep resolving, each
	// through the comm plugin that owns that channel type and nothing the host names itself.
	for _, channelType := range []string{"slack", "discord", "teams", "telegram", "whatsapp", "signal", "ntfy", "gotify"} {
		c, ok := reg.Provider(KindWorkflowNodeAlias, "send-"+channelType)
		if !ok || c.Plugin != "comm-"+channelType || c.ChannelType != channelType {
			t.Errorf("stored send-%s: want comm-%s's %s channel, got %+v (%v)", channelType, channelType, channelType, c, ok)
		}
	}
	if len(reg.EmailChannelTypes()) != 3 {
		t.Errorf("email-channel: want exactly smtp, gmail and exchange, got %v", reg.EmailChannelTypes())
	}
	if c, ok := reg.Provider(KindPickerSource, "vm"); !ok || c.Picker == nil || c.Picker.Permission != "vm.view" {
		t.Errorf("vm picker: want a resolved list gated on vm.view, got %+v", c.Picker)
	}
	exports := reg.OfKind(KindDocumentExport)
	if len(exports) != 1 || exports[0].Plugin != "addon-reporting" || !strings.Contains(strings.Join(exports[0].Formats, ","), "pdf") {
		t.Errorf("document-export: want addon-reporting with pdf, got %+v", exports)
	} else if exports[0].GeneratePath == "" || exports[0].ExportPath == "" {
		t.Errorf("document-export: addon-reporting must carry its generate and export routes, got %+v", exports[0])
	}
}

// TestParseSharedCases runs the accept/refuse set the CLI's `plugin dev validate` tests
// run too (src/contracts/plugincaps/cases.yaml), so the two rule sets cannot drift.
func TestParseSharedCases(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "plugincaps", "cases.yaml"))
	if err != nil {
		t.Fatalf("shared fixtures unreadable: %v", err)
	}
	var set struct {
		Manifest string `yaml:"manifest"`
		Cases    []struct {
			Name         string `yaml:"name"`
			Error        string `yaml:"error"`
			Capabilities string `yaml:"capabilities"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	if len(set.Cases) < 139 {
		t.Fatalf("shared fixtures shrank to %d cases; the parity guard only grows", len(set.Cases))
	}
	for _, c := range set.Cases {
		t.Run(c.Name, func(t *testing.T) {
			lines := strings.Split(strings.TrimRight(c.Capabilities, "\n"), "\n")
			for i, l := range lines {
				lines[i] = "    " + l
			}
			_, err := Parse("addon-demo", []byte(set.Manifest+strings.Join(lines, "\n")+"\n"))
			if c.Error == "" {
				if err != nil {
					t.Fatalf("want a clean manifest, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.Error) {
				t.Fatalf("want ErrInvalid containing %q, got %v", c.Error, err)
			}
		})
	}
}
