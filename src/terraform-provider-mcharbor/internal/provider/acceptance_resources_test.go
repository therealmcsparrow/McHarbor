// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accGet reads one endpoint of the instance under test directly, for the ids a
// test needs before it can write its configuration (a user, a local environment).
func accGet(t *testing.T, path string, out any) {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	if os.Getenv("MCHARBOR_INSECURE_SKIP_VERIFY") != "" {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(os.Getenv(EnvEndpoint), "/")+path, nil)
	if err != nil {
		t.Fatalf("building %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(EnvAPIKey))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || !env.Success {
		t.Fatalf("GET %s: status %d, decode err %v", path, resp.StatusCode, err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("GET %s: data: %v", path, err)
	}
}

// accLocalEnvironmentID returns the socket environment the instance manages
// itself through, which is the one place a test stack can really deploy.
func accLocalEnvironmentID(t *testing.T) string {
	t.Helper()
	var envs []struct {
		ID             string `json:"id"`
		ConnectionType string `json:"connectionType"`
	}
	accGet(t, "/api/environments", &envs)
	for _, e := range envs {
		if e.ConnectionType == "socket" {
			return e.ID
		}
	}
	t.Skip("no socket environment on this instance; stack acceptance needs one")
	return ""
}

// accUserID resolves a username to its id for the role assignment test.
func accUserID(t *testing.T, username string) string {
	t.Helper()
	var page struct {
		Items []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"items"`
	}
	accGet(t, "/api/users?per_page=1000", &page)
	for _, u := range page.Items {
		if u.Username == username {
			return u.ID
		}
	}
	t.Skipf("user %q not found", username)
	return ""
}

func accUsername() string {
	if u := os.Getenv("MCHARBOR_ACC_USER"); u != "" {
		return u
	}
	return "admin"
}

// TestAccEnvironment covers a TCP environment through create, rename, import,
// and an agent environment whose one-time install token must reach state.
func TestAccEnvironment(t *testing.T) {
	requireAcc(t)
	cfg := func(name string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_environment" "tcp" {
  name            = %q
  connection_type = "tcp"
  host            = "192.0.2.10"
  port            = 2375
}

resource "mcharbor_environment" "agent" {
  name            = "tf-acc-agent"
  connection_type = "agent"
}
`, name)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("tf-acc-tcp"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mcharbor_environment.tcp", "id"),
					resource.TestCheckResourceAttr("mcharbor_environment.tcp", "port", "2375"),
					resource.TestCheckResourceAttrSet("mcharbor_environment.agent", "agent_token"),
				),
			},
			{
				Config: cfg("tf-acc-tcp-renamed"),
				Check:  resource.TestCheckResourceAttr("mcharbor_environment.tcp", "name", "tf-acc-tcp-renamed"),
			},
			{
				ResourceName:            "mcharbor_environment.tcp",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"agent_token", "ssh_key", "tls_ca", "tls_cert", "tls_key"},
			},
		},
	})
}

// TestAccStack deploys a one-container stack on the local environment, changes
// its description in place, and imports it by <environment_id>/<name>.
func TestAccStack(t *testing.T) {
	requireAcc(t)
	envID := accLocalEnvironmentID(t)
	cfg := func(description string) string {
		return accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_stack" "test" {
  name           = "tf-acc-stack"
  environment_id = %q
  description    = %q
  compose        = <<-EOT
    services:
      sleeper:
        image: busybox:1.36
        command: ["sleep", "3600"]
  EOT
  env_vars = {
    GREETING = "hello"
  }
}
`, envID, description)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("created by the provider acceptance test"),
				Check:  resource.TestCheckResourceAttr("mcharbor_stack.test", "name", "tf-acc-stack"),
			},
			{
				Config: cfg("updated by the provider acceptance test"),
				Check:  resource.TestCheckResourceAttr("mcharbor_stack.test", "description", "updated by the provider acceptance test"),
			},
			{
				ResourceName:      "mcharbor_stack.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["mcharbor_stack.test"]
					return rs.Primary.Attributes["environment_id"] + "/" + rs.Primary.Attributes["name"], nil
				},
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"env_vars"},
			},
		},
	})
}

// TestAccRoleAssignmentAndAPIKey grants a fresh role to a user and scopes an API
// key to it, then imports both, which is the governance-as-code case end to end.
func TestAccRoleAssignmentAndAPIKey(t *testing.T) {
	requireAcc(t)
	userID := accUserID(t, accUsername())
	cfg := accProviderBlock() + fmt.Sprintf(`
resource "mcharbor_role" "viewer" {
  name        = "tf-acc-viewer"
  description = "acceptance test role"
  permissions = ["containers.view"]
}

resource "mcharbor_role_assignment" "grant" {
  user_id = %q
  role_id = mcharbor_role.viewer.id
}

resource "mcharbor_api_key" "ci" {
  name     = "tf-acc-key"
  role_ids = [mcharbor_role.viewer.id]
}
`, userID)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mcharbor_role_assignment.grant", "id"),
					resource.TestCheckResourceAttrSet("mcharbor_api_key.ci", "key"),
					resource.TestCheckResourceAttrSet("mcharbor_api_key.ci", "key_prefix"),
				),
			},
			{
				ResourceName:      "mcharbor_role_assignment.grant",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["mcharbor_role_assignment.grant"]
					return rs.Primary.Attributes["user_id"] + "/" + rs.Primary.ID, nil
				},
			},
			{
				ResourceName:            "mcharbor_api_key.ci",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"key"},
			},
		},
	})
}

// TestAccListDataSources reads the environment and role lists a module would
// look ids up in.
func TestAccListDataSources(t *testing.T) {
	requireAcc(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accFactories,
		Steps: []resource.TestStep{{
			Config: accProviderBlock() + `
data "mcharbor_environments" "all" {}
data "mcharbor_roles" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.mcharbor_environments.all", "environments.0.id"),
				resource.TestCheckResourceAttrSet("data.mcharbor_roles.all", "roles.0.id"),
			),
		}},
	})
}
