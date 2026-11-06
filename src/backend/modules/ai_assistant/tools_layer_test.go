// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"regexp"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

type allowAll struct{}

func (allowAll) HasPermission(context.Context, string, string, rbac.Permission) (bool, error) {
	return true, nil
}

var (
	providerToolName     = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	providerPropertyName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
)

// TestEveryToolIsAcceptableToTheProviders guards a failure that takes down the whole
// chat: one invalid tool definition makes the provider refuse every message with a 400,
// while connection checks and tool-less calls keep working.
func TestEveryToolIsAcceptableToTheProviders(t *testing.T) {
	tools := BuildPluginTools(context.Background(), allowAll{}, "user", "env")
	if len(tools) == 0 {
		t.Fatal("no tools in the catalog, so this test checks nothing")
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if !providerToolName.MatchString(tool.Name) {
			t.Errorf("tool name %q breaks the provider naming rule", tool.Name)
		}
		if seen[tool.Name] {
			t.Errorf("two tools share the wire name %q", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Parameters["type"] != "object" {
			t.Errorf("%s: input schema type is %v, want object", tool.Name, tool.Parameters["type"])
		}
		if props, ok := tool.Parameters["properties"].(map[string]any); ok {
			for key := range props {
				if !providerPropertyName.MatchString(key) {
					t.Errorf("%s: property %q breaks the provider naming rule", tool.Name, key)
				}
			}
		}
	}
}

// TestWireNamesLeadBackToTheRegisteredTool: a call the model makes must reach the tool
// it was offered, and a model repeating the dotted form still resolves.
func TestWireNamesLeadBackToTheRegisteredTool(t *testing.T) {
	names := ai_tools.Names()
	if len(names) == 0 {
		t.Fatal("no registered tools")
	}
	for _, name := range names {
		if got := canonicalToolName(toolWireName(name)); got != name {
			t.Errorf("wire name of %q leads to %q", name, got)
		}
		if got := canonicalToolName(name); got != name {
			t.Errorf("dotted %q leads to %q", name, got)
		}
	}
}
