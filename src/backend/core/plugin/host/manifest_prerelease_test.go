// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

// TestManifestFloorRange runs the shared floor cases through the manifest schema,
// so a prerelease floor passes while a hostile suffix stays impossible.
func TestManifestFloorRange(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/pluginmanifest/floor_cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Range []struct {
			Floor string `yaml:"floor"`
			Valid bool   `yaml:"valid"`
		} `yaml:"range"`
	}
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Range) == 0 {
		t.Fatal("no range cases loaded")
	}
	for _, c := range cases.Range {
		m := &contract.Manifest{Slug: "demo-plugin", Name: "Demo", Version: "2026.11.02", Author: "x", Mcharbor: c.Floor}
		err := ValidateManifestSchema(m)
		if c.Valid && err != nil {
			t.Errorf("floor %q rejected: %v", c.Floor, err)
		}
		if !c.Valid && err == nil {
			t.Errorf("floor %q accepted", c.Floor)
		}
	}
}
