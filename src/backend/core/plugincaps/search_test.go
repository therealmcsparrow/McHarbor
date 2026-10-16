// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const searchManifest = `
slug: addon-demo
adds_permissions:
  - key: "demo.view"
  - key: "demo.manage"
contributes:
  routes:
    - method: GET
      path: "/api/demo/machines"
      requires_permission: "demo.view"
  capabilities:
    - kind: search-source
      i18n_ns: demo
      sources:
        - {key: vm, label_key: search.group.vms, path: "/api/demo/machines", env_scoped: true, id: id, name: [name], detail: [state], link: "/plugins/addon-demo/{id}", icon: device-desktop}
    - kind: search-action
      i18n_ns: demo
      actions:
        - {key: vm.create, label_key: search.action.createVm, icon: plus, link: "/plugins/addon-demo?action=create", permission: demo.manage}
`

// TestParseResolvesSearchEntriesWithTheirGateAndNamespace: the palette gates a source on the
// key its list route enforces and translates every heading in the plugin's namespace, and
// neither may come from what the manifest claims about itself.
func TestParseResolvesSearchEntriesWithTheirGateAndNamespace(t *testing.T) {
	caps, err := Parse("addon-demo", []byte(searchManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var src *SearchSource
	var act *SearchAction
	for _, c := range caps {
		if c.Kind == KindSearchSource && len(c.Sources) == 1 {
			src = &c.Sources[0]
		}
		if c.Kind == KindSearchAction && len(c.Actions) == 1 {
			act = &c.Actions[0]
		}
	}
	if src == nil || act == nil {
		t.Fatalf("want one source and one action, got %+v", caps)
	}
	if src.Permission != "demo.view" || src.I18nNS != "demo" || !src.EnvScoped {
		t.Errorf("source: want the route's key, the namespace and env scoping, got %+v", src)
	}
	if act.Permission != "demo.manage" || act.I18nNS != "demo" {
		t.Errorf("action: want its key and the namespace, got %+v", act)
	}
	raw, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"sources":[{`, `"envScoped":true`, `"labelKey":"search.group.vms"`, `"permission":"demo.view"`, `"i18nNs":"demo"`, `"actions":[{`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the loader JSON lacks %s: %s", want, raw)
		}
	}
}

func TestSearchSourcePermissionIsNeverReadFromTheManifest(t *testing.T) {
	m := strings.Replace(searchManifest, "icon: device-desktop}", "icon: device-desktop, permission: other.view}", 1)
	caps, err := Parse("addon-demo", []byte(m))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, c := range caps {
		for _, s := range c.Sources {
			if s.Permission != "demo.view" {
				t.Errorf("a source must be gated on its route's key, got %q", s.Permission)
			}
		}
	}
}

func TestSearchLinkRefusesEverythingOutsideThePluginsOwnPages(t *testing.T) {
	for _, link := range []string{
		"//evil.example/plugins/addon-demo", "/plugins/addon-demo/./x", "/plugins/addon-demo//x",
		"/plugins/addon-demo/x?a=1&", "/plugins/addon-demo/%2e%2e", "/plugins/addon-demo\\x", "plugins/addon-demo",
	} {
		if err := searchLink("addon-demo", link, true); err == nil {
			t.Errorf("link %q must be refused", link)
		}
	}
	for _, link := range []string{"/plugins/addon-demo", "/plugins/addon-demo/pools/{id}", "/plugins/addon-demo?action=create&kind=vm"} {
		if err := searchLink("addon-demo", link, true); err != nil {
			t.Errorf("link %q refused: %v", link, err)
		}
	}
}

// TestSearchIconsMatchTheSharedContract keeps this list, the CLI's and the palette's icon map
// on src/contracts/plugincaps/search_icons.yaml.
func TestSearchIconsMatchTheSharedContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "plugincaps", "search_icons.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Icons []string `yaml:"icons"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Icons, SearchIcons) {
		t.Errorf("SearchIcons = %v, contract = %v", SearchIcons, doc.Icons)
	}
}

// TestShippedSearchDeclarations is the contract over the five compute and orchestration
// plugins: each declares the palette groups and actions the approved design gave it.
func TestShippedSearchDeclarations(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	want := map[string]struct{ sources, actions []string }{
		"addon-kvm":        {[]string{"vm"}, []string{"vm.create", "vm.import"}},
		"addon-lxc":        {[]string{"ct"}, []string{"ct.create"}},
		"addon-compute":    {[]string{"host", "pool", "image"}, []string{"compute.host.setup", "compute.pool.create", "compute.image.upload"}},
		"addon-kubernetes": {[]string{"deployment", "pod"}, []string{"k8s.cluster.add"}},
		"addon-swarm":      {[]string{"service"}, []string{"swarm.stack.deploy"}},
	}
	for slug, w := range want {
		raw, err := os.ReadFile(filepath.Join(root, slug, "plugin.yaml"))
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		caps, err := Parse(slug, raw)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		var sources, actions []string
		for _, c := range caps {
			for _, s := range c.Sources {
				sources = append(sources, s.Key)
				if s.Permission == "" || s.I18nNS == "" {
					t.Errorf("%s source %s: unresolved gate or namespace %+v", slug, s.Key, s)
				}
				requireLabel(t, filepath.Join(root, slug), s.I18nNS, s.LabelKey)
			}
			for _, a := range c.Actions {
				actions = append(actions, a.Key)
				requireLabel(t, filepath.Join(root, slug), a.I18nNS, a.LabelKey)
				if a.KeywordsKey != "" {
					requireLabel(t, filepath.Join(root, slug), a.I18nNS, a.KeywordsKey)
				}
			}
		}
		if !slices.Equal(sources, w.sources) {
			t.Errorf("%s sources = %v, want %v", slug, sources, w.sources)
		}
		if !slices.Equal(actions, w.actions) {
			t.Errorf("%s actions = %v, want %v", slug, actions, w.actions)
		}
	}
}

// requireLabel fails unless key resolves to a string in the plugin's ns bundle in every
// locale, since the palette renders a missing heading as its raw dotted key.
func requireLabel(t *testing.T, pluginDir, ns, key string) {
	t.Helper()
	for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
		raw, err := os.ReadFile(filepath.Join(pluginDir, "i18n", lang, ns+".json"))
		if err != nil {
			t.Errorf("%s: %v", pluginDir, err)
			continue
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			t.Errorf("%s/%s: %v", pluginDir, lang, err)
			continue
		}
		for _, part := range strings.Split(key, ".") {
			m, _ := node.(map[string]any)
			node = m[part]
		}
		if v, ok := node.(string); !ok || v == "" {
			t.Errorf("%s: %s.%s is missing from the %s bundle", filepath.Base(pluginDir), ns, key, lang)
		}
	}
}

// TestKubernetesSearchFansOutPerCluster pins K1: both Kubernetes groups are requested once
// per cluster, gated by the cluster list's own key, and link back with the cluster.
func TestKubernetesSearchFansOutPerCluster(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "Plugins", "addon-kubernetes", "plugin.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("plugin source tree not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	caps, err := Parse("addon-kubernetes", raw)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, c := range caps {
		for _, s := range c.Sources {
			seen++
			if s.FanOut == nil || s.FanOut.Param != "cluster" || s.FanOut.Path != "/api/kubernetes/clusters" {
				t.Errorf("source %s does not fan out per cluster: %+v", s.Key, s.FanOut)
				continue
			}
			if s.FanOut.Permission == "" {
				t.Errorf("source %s: the cluster list's gate was not resolved", s.Key)
			}
			if !strings.HasSuffix(s.Link, "?cluster={cluster}") {
				t.Errorf("source %s link %q does not carry the cluster", s.Key, s.Link)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("%d Kubernetes sources, want 2", seen)
	}
}
