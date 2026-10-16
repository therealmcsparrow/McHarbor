// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSearchIconsMatchTheSharedContract holds the CLI's icon list to the one the host and
// the palette are held to, so a manifest the CLI passes is never refused at load.
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
	if !slices.Equal(doc.Icons, pluginSearchIcons) {
		t.Errorf("pluginSearchIcons = %v, contract = %v", pluginSearchIcons, doc.Icons)
	}
}
