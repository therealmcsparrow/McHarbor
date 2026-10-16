// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWorkflowFromFile covers create from a file, a formatting-only edit
// that must plan nothing, an in-place status change, and import.
func TestAccWorkflowFromFile(t *testing.T) {
	requireAcc(t)
	dir := t.TempDir()
	file := filepath.ToSlash(filepath.Join(dir, "wf.json"))
	write := func(body string) {
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pretty := `{
  "nodes": [
    { "id": "t", "type": "trigger", "action": "schedule-trigger", "label": "Nightly", "config": { "cron": "0 3 * * *" } },
    { "id": "d", "type": "action", "action": "debug", "label": "Log", "config": {} }
  ],
  "edges": [ { "id": "e", "sourceNodeId": "t", "sourcePort": "output", "targetNodeId": "d", "targetPort": "input" } ]
}`
	cfg := func(status string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_workflow" "w" {
  name        = "tf-acc-workflow"
  status      = %q
  canvas_json = file(%q)
}
`, status, file)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { write(pretty) },
				Config:    cfg("draft"),
				Check:     resource.TestCheckResourceAttr("mcharbor_workflow.w", "status", "draft"),
			},
			{
				PreConfig: func() { write(compactJSON(pretty)) },
				Config:    cfg("draft"),
				PlanOnly:  true,
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

// TestAccPluginAndInventory installs a plugin at the catalog's latest version,
// imports it, and reads every Phase 3 data source against the live instance.
func TestAccPluginAndInventory(t *testing.T) {
	requireAcc(t)
	slug := os.Getenv("MCHARBOR_ACC_PLUGIN")
	if slug == "" {
		t.Skip("set MCHARBOR_ACC_PLUGIN to a catalog plugin slug the instance may install and remove")
	}
	cfg := accProviderBlock() + fmt.Sprintf(`
data "mcharbor_plugin_catalog" "all" {}
data "mcharbor_containers" "all" {}
data "mcharbor_images" "all" {}
data "mcharbor_app_catalog" "all" {}

resource "mcharbor_plugin" "p" {
  slug    = %q
  version = one([for p in data.mcharbor_plugin_catalog.all.plugins : p.latest_version if p.slug == %q])
  enabled = false
}
`, slug, slug)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_plugin.p", "id", slug),
					resource.TestCheckResourceAttr("mcharbor_plugin.p", "enabled", "false"),
					resource.TestCheckResourceAttrSet("data.mcharbor_plugin_catalog.all", "plugins.#"),
					resource.TestCheckResourceAttrSet("data.mcharbor_containers.all", "containers.#"),
					resource.TestCheckResourceAttrSet("data.mcharbor_images.all", "images.#"),
					resource.TestCheckResourceAttrSet("data.mcharbor_app_catalog.all", "apps.#"),
				),
			},
			{
				ResourceName:            "mcharbor_plugin.p",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"purge_on_destroy"},
			},
		},
	})
}
