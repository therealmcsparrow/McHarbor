// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// checkNoSecretValue fails when any attribute of the resource holds the value,
// the one thing the secret design promises never reaches state.
func checkNoSecretValue(name, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		for k, v := range rs.Primary.Attributes {
			if strings.Contains(v, value) {
				return fmt.Errorf("secret value found in state attribute %q", k)
			}
		}
		if _, set := rs.Primary.Attributes["value_wo"]; set {
			return fmt.Errorf("value_wo present in state")
		}
		return nil
	}
}

// TestAccSecretWriteOnly covers option A: a write-only value, rotation by
// bumping value_wo_version, and import — with the value absent from state throughout.
func TestAccSecretWriteOnly(t *testing.T) {
	requireAcc(t)
	cfg := func(value string, version int) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_secret" "a" {
  name             = "TF_ACC_SECRET_A"
  description      = "acceptance test, option A"
  kind             = "password"
  value_wo         = %q
  value_wo_version = %d
}

data "mcharbor_secret" "a" {
  name  = mcharbor_secret.a.name
  scope = "global"
}
`, value, version)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("first-value-tf-acc-7c1d", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_secret.a", "version", "1"),
					resource.TestCheckResourceAttr("mcharbor_secret.a", "generated", "false"),
					checkNoSecretValue("mcharbor_secret.a", "first-value-tf-acc-7c1d"),
					resource.TestCheckResourceAttrPair("data.mcharbor_secret.a", "id", "mcharbor_secret.a", "id"),
				),
			},
			{
				Config: cfg("second-value-tf-acc-9e2f", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_secret.a", "version", "2"),
					checkNoSecretValue("mcharbor_secret.a", "second-value-tf-acc-9e2f"),
				),
			},
			{
				ResourceName:            "mcharbor_secret.a",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value_wo_version", "generate_length"},
			},
		},
	})
}

// TestAccSecretGenerated covers option B: McHarbor mints the value and the
// provider discards it.
func TestAccSecretGenerated(t *testing.T) {
	requireAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: accProviderBlock() + `
resource "mcharbor_secret" "b" {
  name            = "TF_ACC_SECRET_B"
  kind            = "token"
  generate        = true
  generate_length = 32
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_secret.b", "generated", "true"),
					resource.TestCheckNoResourceAttr("mcharbor_secret.b", "value_wo"),
				),
			},
			{
				ResourceName:            "mcharbor_secret.b",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"generate", "generate_length", "value_wo_version"},
			},
		},
	})
}

// TestAccScheduleAndUpdatePolicy creates both disabled, with targets that match
// nothing, so the test can never act on a real container.
func TestAccScheduleAndUpdatePolicy(t *testing.T) {
	requireAcc(t)
	envID := accLocalEnvironmentID(t)
	cfg := func(cron string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_schedule" "s" {
  name           = "tf-acc-schedule"
  cron           = %q
  action         = "restart"
  target         = "tf-acc-no-such-container"
  environment_id = %q
  enabled        = false
}

resource "mcharbor_update_policy" "p" {
  name            = "tf-acc-policy"
  container_match = "tf-acc-no-such-*"
  schedule        = %q
  strategy        = "digest"
  enabled         = false
}
`, cron, envID, cron)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("0 3 1 1 *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_schedule.s", "enabled", "false"),
					resource.TestCheckResourceAttr("mcharbor_update_policy.p", "enabled", "false"),
				),
			},
			{
				Config: cfg("0 4 1 1 *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_schedule.s", "cron", "0 4 1 1 *"),
					resource.TestCheckResourceAttr("mcharbor_update_policy.p", "schedule", "0 4 1 1 *"),
				),
			},
			{ResourceName: "mcharbor_schedule.s", ImportState: true, ImportStateVerify: true},
			{ResourceName: "mcharbor_update_policy.p", ImportState: true, ImportStateVerify: true},
		},
	})
}

// TestAccAlertRule uses a threshold on a container that does not exist, so the
// rule can never fire while the test runs.
func TestAccAlertRule(t *testing.T) {
	requireAcc(t)
	cfg := func(threshold string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_alert_rule" "r" {
  name          = "tf-acc-alert"
  source_type   = "container"
  condition_key = "cpu"
  condition = {
    operator  = ">"
    threshold = %s
  }
  target      = "tf-acc-no-such-container"
  send_in_app = true
  enabled     = false
}
`, threshold)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("99.5"),
				Check:  resource.TestCheckResourceAttr("mcharbor_alert_rule.r", "condition.threshold", "99.5"),
			},
			{
				Config: cfg("97.25"),
				Check:  resource.TestCheckResourceAttr("mcharbor_alert_rule.r", "condition.threshold", "97.25"),
			},
			{ResourceName: "mcharbor_alert_rule.r", ImportState: true, ImportStateVerify: true},
		},
	})
}

// TestAccStorageLocationAndBackupPlan builds a disabled plan for McHarbor's own
// container onto a disabled OneDrive location that never completes consent; nothing
// is ever archived. McHarbor only accepts location types a storage plugin provides.
func TestAccStorageLocationAndBackupPlan(t *testing.T) {
	requireAcc(t)
	accRequireStorageType(t, "onedrive_business")
	envID := accLocalEnvironmentID(t)
	containerID := accContainerID(t, envID, "mcharbor")
	cfg := func(retention int) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_storage_location" "l" {
  name                = "tf-acc-location"
  location_type       = "onedrive_business"
  tenant_id           = "00000000-0000-0000-0000-000000000000"
  base_path           = "/tf-acc"
  client_id_wo        = "tf-acc-client-id"
  client_secret_wo    = "tf-acc-storage-password-3b8a"
  credentials_version = 1
  enabled             = false
}

resource "mcharbor_backup_plan" "p" {
  name                 = "tf-acc-plan"
  environment_id       = %q
  container_id         = %q
  storage_location_ids = [mcharbor_storage_location.l.id]
  include_config       = true
  cron                 = "0 5 1 1 *"
  enabled              = false
  retention_count      = %d
}
`, envID, containerID, retention)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mcharbor_storage_location.l", "enabled", "false"),
					checkNoSecretValue("mcharbor_storage_location.l", "tf-acc-storage-password-3b8a"),
					resource.TestCheckResourceAttr("mcharbor_backup_plan.p", "retention_count", "3"),
				),
			},
			{
				Config: cfg(5),
				Check:  resource.TestCheckResourceAttr("mcharbor_backup_plan.p", "retention_count", "5"),
			},
			{ResourceName: "mcharbor_storage_location.l", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"credentials_version"}},
			{
				ResourceName:      "mcharbor_backup_plan.p",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["mcharbor_backup_plan.p"]
					return rs.Primary.Attributes["environment_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}

// accContainerID finds a running container on envID whose name contains part,
// for the backup plan, which must point at a real container.
func accContainerID(t *testing.T, envID, part string) string {
	t.Helper()
	var containers []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
	}
	accGet(t, "/api/containers?env="+envID, &containers)
	for _, c := range containers {
		for _, n := range c.Names {
			if strings.Contains(n, part) {
				return c.ID
			}
		}
	}
	t.Skipf("no container matching %q on the local environment", part)
	return ""
}

// accRequireStorageType skips unless a storage plugin provides the type, since
// McHarbor refuses to create a location of a type nothing can serve.
func accRequireStorageType(t *testing.T, locationType string) {
	t.Helper()
	var adapters map[string]any
	accGet(t, "/api/plugins/loader/storage-adapters", &adapters)
	if _, ok := adapters[locationType]; !ok {
		t.Skipf("no storage plugin provides %q on this instance", locationType)
	}
}
