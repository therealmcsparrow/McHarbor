// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

// fakePluginSource stands in for the plugin runtime. It satisfies
// host.AlertSourceAdapterSource structurally, which is the same way the
// real runtime satisfies it.
type fakePluginSource struct {
	spec   alerting.AlertSourceAdapterSpec
	client alerting.AlertSourceAdapterClient
}

func (f *fakePluginSource) AlertSourceAdapter(sourceType string) (alerting.AlertSourceAdapterClient, alerting.AlertSourceAdapterSpec, bool) {
	if sourceType != f.spec.SourceType {
		return nil, alerting.AlertSourceAdapterSpec{}, false
	}
	return f.client, f.spec, true
}

func (f *fakePluginSource) AlertSourceAdapters() []alerting.AlertSourceAdapterSpec {
	return []alerting.AlertSourceAdapterSpec{f.spec}
}

type fakeClient struct{}

func (fakeClient) Evaluate(context.Context, alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	return nil, nil
}

// installFakePluginSource wires a plugin source into the process-wide
// registry and removes it afterwards, so one test cannot leave a source
// visible to the next.
func installFakePluginSource(t *testing.T, spec alerting.AlertSourceAdapterSpec) {
	t.Helper()

	registry.setPluginSource(&fakePluginSource{spec: spec, client: fakeClient{}})
	t.Cleanup(func() {
		registry.mu.Lock()
		registry.pluginSource = nil
		registry.mu.Unlock()
	})
}

func kubernetesLikeSpec() alerting.AlertSourceAdapterSpec {
	return alerting.AlertSourceAdapterSpec{
		SourceType:  "kubernetes",
		DisplayName: "Kubernetes",
		Slug:        "kubernetes",
		Conditions: []alerting.AlertConditionDecl{{
			Key:         "pod_crashloop",
			DisplayName: "Pod in CrashLoopBackOff",
			Kind:        alerting.KindEvent,
			TargetKind:  alerting.TargetNone,
		}},
	}
}

func TestCatalogIncludesPluginSources(t *testing.T) {
	installFakePluginSource(t, kubernetesLikeSpec())

	catalog := registry.catalog()

	var found *CatalogSource
	for i := range catalog.Sources {
		if catalog.Sources[i].SourceType == "kubernetes" {
			found = &catalog.Sources[i]
		}
	}
	if found == nil {
		t.Fatal("expected a plugin-contributed source in the catalog")
	}
	if found.BuiltIn {
		t.Fatal("a source carrying a plugin slug must not be reported as built-in")
	}
	if len(found.Conditions) != 1 {
		t.Fatalf("expected 1 plugin condition, got %d", len(found.Conditions))
	}

	// A plugin ships its own localised strings, so the host must not
	// hand the frontend a key it has nothing behind.
	if found.Conditions[0].I18nKey != "" {
		t.Fatalf("plugin conditions must carry no host i18n key, got %q", found.Conditions[0].I18nKey)
	}

	// Built-ins must still be there, and still be first.
	if len(catalog.Sources) < 4 || !catalog.Sources[0].BuiltIn {
		t.Fatal("expected built-in sources to lead the catalog")
	}
}

func TestResolvePluginSource(t *testing.T) {
	installFakePluginSource(t, kubernetesLikeSpec())

	if _, _, ok := registry.resolve("kubernetes"); !ok {
		t.Fatal("expected the plugin source to resolve")
	}
	if !registry.available("kubernetes") {
		t.Fatal("expected the plugin source to report as available")
	}

	decl, ok := registry.condition("kubernetes", "pod_crashloop")
	if !ok {
		t.Fatal("expected the plugin condition to resolve")
	}
	if decl.Kind != alerting.KindEvent {
		t.Fatalf("unexpected kind %q", decl.Kind)
	}
}

// A rule whose plugin was uninstalled must resolve to nothing and be
// reported unavailable — never deleted, because reinstalling the plugin
// has to bring the rule back exactly as it was.
func TestUninstalledPluginSourceIsUnavailableNotFatal(t *testing.T) {
	if _, _, ok := registry.resolve("kubernetes"); ok {
		t.Fatal("expected no plugin source with none installed")
	}
	if registry.available("kubernetes") {
		t.Fatal("expected an uninstalled source to report unavailable")
	}
	if _, ok := registry.condition("kubernetes", "pod_crashloop"); ok {
		t.Fatal("expected no condition to resolve with the plugin gone")
	}
}
