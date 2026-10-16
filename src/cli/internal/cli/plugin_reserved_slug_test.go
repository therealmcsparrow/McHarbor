// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestValidateRefusesAReservedSlug: plugin dev validate reports the slugs the host installer
// and loader refuse, read from the contract the backend tests read too, so an author learns
// it before packaging rather than at install.
func TestValidateRefusesAReservedSlug(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "pluginslugs", "reserved.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Reserved []string `yaml:"reserved"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	got := slices.Clone(reservedPluginSlugs)
	slices.Sort(got)
	slices.Sort(contract.Reserved)
	if !slices.Equal(got, contract.Reserved) {
		t.Fatalf("the CLI's reserved slugs drifted from the contract: CLI %v, contract %v", got, contract.Reserved)
	}
	for _, slug := range contract.Reserved {
		var errs []string
		validateManifestShape(slug, pluginManifest{Slug: slug, Schema: "mcharbor.plugin/v1", Version: "2026.09.01", Mcharbor: ">=2026.09.01"},
			func(severity, _, detail string) {
				if severity == "error" {
					errs = append(errs, detail)
				}
			})
		if !strings.Contains(strings.Join(errs, "\n"), "reserved") {
			t.Fatalf("slug %q must be reported as reserved, got %v", slug, errs)
		}
	}
}
