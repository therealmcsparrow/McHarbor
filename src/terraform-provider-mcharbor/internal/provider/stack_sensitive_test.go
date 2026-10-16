// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestStackComposeAndEnvVarsAreSensitive is L3 of the 2026-09-30 review: the compose field
// is read back with reveal, so it carries credentials and must be masked in plan output
// the same way env_vars already was.
func TestStackComposeAndEnvVarsAreSensitive(t *testing.T) {
	var resp resource.SchemaResponse
	newStackResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	for _, name := range []string{"compose", "env_vars"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("mcharbor_stack has no %s attribute", name)
		}
		if !attr.IsSensitive() {
			t.Errorf("mcharbor_stack.%s is not Sensitive; plan output would print the credentials it holds", name)
		}
	}
}
