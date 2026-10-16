// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginslug

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestReservedSlugsMatchTheSharedContract: the CLI's plugin dev validate reads the same
// file, so the host and the CLI cannot disagree about which slugs are taken.
func TestReservedSlugsMatchTheSharedContract(t *testing.T) {
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
	slices.Sort(contract.Reserved)
	if !slices.Equal(contract.Reserved, List()) {
		t.Fatalf("reserved slugs drifted from the contract: host %v, contract %v", List(), contract.Reserved)
	}
	for _, slug := range contract.Reserved {
		for _, spelling := range []string{slug, " " + slug + " ", "HOST"} {
			if spelling == "HOST" && slug != "host" {
				continue
			}
			if err := Check(spelling); !errors.Is(err, ErrReserved) {
				t.Fatalf("%q must be refused as reserved, got %v", spelling, err)
			}
		}
	}
	if err := Check("addon-host"); err != nil {
		t.Fatalf("a slug merely containing a reserved word is not reserved: %v", err)
	}
}
