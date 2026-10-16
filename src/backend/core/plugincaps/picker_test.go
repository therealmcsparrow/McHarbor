// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"strings"
	"testing"
)

const pickerManifest = `
slug: addon-demo
adds_permissions:
  - key: "demo.view"
contributes:
  routes:
    - method: GET
      path: "/api/demo/machines"
      requires_permission: "demo.view"
  nodes:
    - "demo-trigger"
  capabilities:
    - kind: runbook-provider
      domain: vm
      i18n_ns: demo
      targets:
        - {kind: vm, label_key: runbook.targets.vm, path: "/api/demo/machines", query: {env: "$env"}, id: id, name: [name], detail: [state]}
    - kind: picker-source
      target: vm
    - kind: workflow-node-alias
      alias: legacy-trigger
      node: demo-trigger
`

func TestParseResolvesAPickerSourceToItsTargetAndRoutePermission(t *testing.T) {
	caps, err := Parse("addon-demo", []byte(pickerManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var picker *PickerList
	for _, c := range caps {
		if c.Kind == KindPickerSource {
			picker = c.Picker
		}
	}
	if picker == nil {
		t.Fatal("the picker-source carries no resolved list")
	}
	if picker.Path != "/api/demo/machines" || picker.ID != "id" || picker.Query["env"] != "$env" {
		t.Errorf("want the target's route, id and query, got %+v", picker)
	}
	if strings.Join(picker.Name, ",") != "name" || strings.Join(picker.Detail, ",") != "state" {
		t.Errorf("want the target's name and detail fields, got %+v", picker)
	}
	if picker.Permission != "demo.view" || picker.I18nNS != "demo" || picker.LabelKey != "runbook.targets.vm" {
		t.Errorf("want the route's permission and the provider's namespace, got %+v", picker)
	}
}

func TestCheckFieldSourceHoldsANodeToItsOwnPlugin(t *testing.T) {
	caps, err := Parse("addon-demo", []byte(pickerManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := CheckFieldSource("addon-demo", caps, FieldSource{Plugin: "addon-demo", Target: "vm"}); err != nil {
		t.Errorf("own declared source refused: %v", err)
	}
	if err := CheckFieldSource("addon-demo", caps, FieldSource{Target: "vm"}); err != nil {
		t.Errorf("a source without plugin means the registering plugin, refused: %v", err)
	}
	if err := CheckFieldSource("addon-demo", caps, FieldSource{Plugin: "addon-other", Target: "vm"}); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("another plugin's source must be refused, got %v", err)
	}
	if err := CheckFieldSource("addon-demo", caps, FieldSource{Target: "ct"}); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("an undeclared target must be refused, got %v", err)
	}
}

func TestNodeAliasActionResolvesOnlyWhileTheClaimingPluginIsLoaded(t *testing.T) {
	caps, err := Parse("addon-demo", []byte(pickerManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := NewRegistry()
	r.Set("addon-demo", caps)

	got, ok := r.NodeAliasAction("legacy-trigger")
	if !ok || got != "plugin:addon-demo:demo-trigger" {
		t.Fatalf("want plugin:addon-demo:demo-trigger, got %q %v", got, ok)
	}
	if _, ok := r.NodeAliasAction("plugin:addon-demo:demo-trigger"); ok {
		t.Error("a composite action must never be re-resolved")
	}
	if _, ok := r.NodeAliasAction("manual-trigger"); ok {
		t.Error("an unclaimed action must not resolve")
	}

	r.Remove("addon-demo")
	if _, ok := r.NodeAliasAction("legacy-trigger"); ok {
		t.Error("an unloaded plugin's alias must stop resolving")
	}
}
