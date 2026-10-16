// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type fakeInstance struct {
	mu        sync.Mutex
	plugins   map[string]installedPlugin
	catalog   map[string][]string
	purged    map[string]bool
	workflows map[string]map[string]string
	seq       int
	reconcile int
}

func newFakeInstance() *fakeInstance {
	return &fakeInstance{
		plugins:   map[string]installedPlugin{},
		catalog:   map[string][]string{"comm-ntfy": {"2026.09.01", "2026.09.02"}},
		purged:    map[string]bool{},
		workflows: map[string]map[string]string{},
	}
}

func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func writeFail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg, "code": code})
}

func (f *fakeInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.Path
	q := r.URL.Query()
	switch {
	case p == "/api/containers":
		if q.Get("env") != "env1" || q.Get("all") != "true" {
			writeFail(w, http.StatusBadRequest, "ERR_INVALID_BODY", "bad query "+r.URL.RawQuery)
			return
		}
		writeOK(w, []map[string]any{{"Id": "c1", "Names": []string{"/web"}, "Image": "nginx:1", "State": "running", "Status": "Up 2 hours", "StackName": "site"}})
	case p == "/api/images":
		writeOK(w, []map[string]any{{"Id": "sha256:1", "RepoTags": []string{"nginx:1", "nginx:latest"}, "Size": 4096}})
	case p == "/api/app-store":
		if q.Get("category") != "db" || q.Get("per_page") != "1000" {
			writeFail(w, http.StatusBadRequest, "ERR_INVALID_BODY", "bad query "+r.URL.RawQuery)
			return
		}
		writeOK(w, map[string]any{"items": []map[string]any{{"id": "a1", "slug": "postgres", "name": "PostgreSQL", "category": "db", "image": "postgres:17", "description": "SQL"}}, "total": 1})
	case p == "/api/appstore/plugins/catalog":
		writeOK(w, []map[string]any{{"slug": "comm-ntfy", "name": "ntfy", "version": "2026.09.02", "description": "push",
			"versions": []map[string]string{{"version": "2026.09.02"}, {"version": "2026.09.01"}}}})
	case p == "/api/appstore/plugins/uninstall" && r.Method == http.MethodPost:
		var in struct {
			Slug      string `json:"slug"`
			PurgeData bool   `json:"purge_data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if _, ok := f.plugins[in.Slug]; !ok {
			writeFail(w, http.StatusNotFound, "ERR_PLUGIN_NOT_FOUND", "not installed")
			return
		}
		delete(f.plugins, in.Slug)
		f.purged[in.Slug] = in.PurgeData
		writeOK(w, map[string]any{"slug": in.Slug, "status": "uninstalled"})
	case strings.HasSuffix(p, "/reconcile") && strings.HasPrefix(p, "/api/appstore/plugins/"):
		slug := strings.TrimSuffix(strings.TrimPrefix(p, "/api/appstore/plugins/"), "/reconcile")
		var in struct {
			Version string `json:"version"`
			Enabled *bool  `json:"enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		served := false
		for _, v := range f.catalog[slug] {
			served = served || v == in.Version
		}
		if !served {
			writeFail(w, http.StatusConflict, "ERR_PLUGIN_UPDATE_FAILED", "the catalog does not serve "+slug+" "+in.Version)
			return
		}
		f.reconcile++
		f.plugins[slug] = installedPlugin{Slug: slug, Version: in.Version, Enabled: in.Enabled == nil || *in.Enabled, Loaded: true, Signed: true}
		writeOK(w, map[string]any{"slug": slug, "version": in.Version, "action": "installed"})
	case strings.HasPrefix(p, "/api/appstore/plugins/"):
		pl, ok := f.plugins[strings.TrimPrefix(p, "/api/appstore/plugins/")]
		if !ok {
			writeFail(w, http.StatusNotFound, "ERR_PLUGIN_NOT_FOUND", "not installed")
			return
		}
		writeOK(w, pl)
	case p == "/api/workflows" && r.Method == http.MethodPost:
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.seq++
		id := fmt.Sprintf("wf%d", f.seq)
		f.workflows[id] = map[string]string{"id": id, "name": in["name"], "description": in["description"], "status": "draft", "canvasData": "{}", "variables": "{}"}
		writeOK(w, f.workflows[id])
	case strings.HasPrefix(p, "/api/workflows/"):
		id := strings.TrimPrefix(p, "/api/workflows/")
		wf, ok := f.workflows[id]
		if !ok {
			writeFail(w, http.StatusNotFound, "ERR_NOT_FOUND", "no workflow")
			return
		}
		switch r.Method {
		case http.MethodPut:
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			for k, v := range in {
				if k == "canvasData" || k == "variables" {
					v = compactJSON(v)
				}
				wf[k] = v
			}
		case http.MethodDelete:
			delete(f.workflows, id)
		}
		writeOK(w, wf)
	default:
		writeFail(w, http.StatusNotFound, "ERR_NOT_FOUND", "no route "+r.Method+" "+p)
	}
}

func compactJSON(s string) string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func fakeProviderBlock(url string) string {
	return fmt.Sprintf("provider \"mcharbor\" {\n  endpoint = %q\n  api_key = \"mh_test\"\n}\n", url)
}

// TestDataSourcesReadTheInventoryAndCatalogs checks each new data source
// reaches its route with the filters the configuration gave it.
func TestDataSourcesReadTheInventoryAndCatalogs(t *testing.T) {
	srv := httptest.NewServer(newFakeInstance())
	t.Cleanup(srv.Close)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{{
			Config: fakeProviderBlock(srv.URL) + `
data "mcharbor_containers" "c" { environment_id = "env1" }
data "mcharbor_images" "i" { environment_id = "env1" }
data "mcharbor_app_catalog" "a" { category = "db" }
data "mcharbor_plugin_catalog" "p" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.mcharbor_containers.c", "containers.0.name", "web"),
				resource.TestCheckResourceAttr("data.mcharbor_containers.c", "containers.0.stack", "site"),
				resource.TestCheckResourceAttr("data.mcharbor_images.i", "images.0.tags.#", "2"),
				resource.TestCheckResourceAttr("data.mcharbor_images.i", "images.0.size", "4096"),
				resource.TestCheckResourceAttr("data.mcharbor_app_catalog.a", "apps.0.slug", "postgres"),
				resource.TestCheckResourceAttr("data.mcharbor_plugin_catalog.p", "plugins.0.latest_version", "2026.09.02"),
				resource.TestCheckResourceAttr("data.mcharbor_plugin_catalog.p", "plugins.0.versions.#", "2"),
			),
		}},
	})
}

// TestPluginPinsAVersionAndKeepsDataByDefault covers install, a version move,
// an idle re-plan, import, and a destroy that leaves the plugin's data alone.
func TestPluginPinsAVersionAndKeepsDataByDefault(t *testing.T) {
	fake := newFakeInstance()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cfg := func(version string) string {
		return fakeProviderBlock(srv.URL) + fmt.Sprintf(`
resource "mcharbor_plugin" "n" {
  slug    = "comm-ntfy"
  version = %q
  enabled = false
}
`, version)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		CheckDestroy: func(*terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if _, ok := fake.plugins["comm-ntfy"]; ok {
				return fmt.Errorf("plugin still installed after destroy")
			}
			if fake.purged["comm-ntfy"] {
				return fmt.Errorf("destroy purged data without purge_on_destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg("2026.09.01"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_plugin.n", "id", "comm-ntfy"),
					resource.TestCheckResourceAttr("mcharbor_plugin.n", "version", "2026.09.01"),
					resource.TestCheckResourceAttr("mcharbor_plugin.n", "enabled", "false"),
					resource.TestCheckResourceAttr("mcharbor_plugin.n", "signed", "true"),
				),
			},
			{
				Config: cfg("2026.09.02"),
				Check:  resource.TestCheckResourceAttr("mcharbor_plugin.n", "version", "2026.09.02"),
			},
			{
				ResourceName:            "mcharbor_plugin.n",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"purge_on_destroy"},
			},
		},
	})
}

// TestPluginVersionTheCatalogLacksFailsByName checks the server's reason
// reaches the operator instead of a generic failure.
func TestPluginVersionTheCatalogLacksFailsByName(t *testing.T) {
	srv := httptest.NewServer(newFakeInstance())
	t.Cleanup(srv.Close)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{{
			Config: fakeProviderBlock(srv.URL) + `
resource "mcharbor_plugin" "n" {
  slug    = "comm-ntfy"
  version = "2026.09.09"
}
`,
			ExpectError: regexp.MustCompile(`comm-ntfy 2026\.09\.09`),
		}},
	})
}

// TestWorkflowIgnoresJSONFormatting checks a pretty-printed canvas the server
// stores compacted never shows up as a change on the next plan.
func TestWorkflowIgnoresJSONFormatting(t *testing.T) {
	srv := httptest.NewServer(newFakeInstance())
	t.Cleanup(srv.Close)
	pretty := `<<-EOT
    {
      "nodes": [ { "id": "n1", "type": "cron" } ],
      "edges": []
    }
  EOT`
	compact := `"{\"edges\":[],\"nodes\":[{\"type\":\"cron\",\"id\":\"n1\"}]}"`
	cfgWith := func(status, canvas string) string {
		return fakeProviderBlock(srv.URL) + fmt.Sprintf(`
resource "mcharbor_workflow" "w" {
  name        = "nightly"
  status      = %q
  canvas_json = %s
}
`, status, canvas)
	}
	cfg := func(status string) string { return cfgWith(status, pretty) }
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("draft"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_workflow.w", "id", "wf1"),
					resource.TestCheckResourceAttr("mcharbor_workflow.w", "variables_json", "{}"),
				),
			},
			{
				Config:   cfgWith("draft", compact),
				PlanOnly: true,
			},
			{
				Config: cfg("active"),
				Check:  resource.TestCheckResourceAttr("mcharbor_workflow.w", "status", "active"),
			},
			{
				ResourceName:            "mcharbor_workflow.w",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"canvas_json"},
			},
		},
	})
}
