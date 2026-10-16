// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var accFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"mcharbor": providerserver.NewProtocol6WithError(New("acc")()),
}

// requireAcc skips unless an instance was provided, so go test ./... stays
// hermetic and the acceptance suite runs only where it can reach McHarbor.
func requireAcc(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
	if os.Getenv(EnvEndpoint) == "" || os.Getenv(EnvAPIKey) == "" {
		t.Skipf("set %s and %s to run acceptance tests", EnvEndpoint, EnvAPIKey)
	}
}

func accProviderBlock() string {
	insecure := os.Getenv("MCHARBOR_INSECURE_SKIP_VERIFY") != ""
	return fmt.Sprintf("provider \"mcharbor\" {\n  insecure_skip_verify = %t\n}\n", insecure)
}

// TestAccRole covers create, an empty follow-up plan, update in place, and import.
func TestAccRole(t *testing.T) {
	requireAcc(t)
	cfg := func(perms string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_role" "test" {
  name        = "tf-acc-role"
  description = "created by the provider acceptance test"
  permissions = [%s]
}
`, perms)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`"containers.view"`),
				Check:  resource.TestCheckResourceAttr("mcharbor_role.test", "permissions.#", "1"),
			},
			{
				Config: cfg(`"containers.view", "images.view"`),
				Check:  resource.TestCheckResourceAttr("mcharbor_role.test", "permissions.#", "2"),
			},
			{
				ResourceName:      "mcharbor_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPermissionCatalog reads the catalog the role editor uses.
func TestAccPermissionCatalog(t *testing.T) {
	requireAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{{
			Config: accProviderBlock() + `data "mcharbor_permission_catalog" "all" {}`,
			Check:  resource.TestCheckResourceAttrSet("data.mcharbor_permission_catalog.all", "permissions.0.key"),
		}},
	})
}
