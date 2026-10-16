// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capacity

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/forecast"
)

// The amber and red lines exist twice, once per language, because the component
// that draws a storage bar sits behind settings.manage and must not have to make a
// gated API call to colour a div. This test is what makes the pair one home rather
// than two numbers that agree today: it fails the moment either side moves.
func TestFrontendThresholdsMatchTheGoConstants(t *testing.T) {
	path := filepath.Join("..", "..", "..", "frontend", "resources", "utils", "capacity.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v — the frontend half of the threshold pair must exist", path, err)
	}
	for _, c := range []struct {
		constant string
		want     float64
	}{
		{"CAPACITY_AMBER_PERCENT", forecast.AmberPercent},
		{"CAPACITY_RED_PERCENT", forecast.RedPercent},
	} {
		re := regexp.MustCompile(`(?m)^export const ` + c.constant + ` = ([0-9.]+);`)
		match := re.FindSubmatch(raw)
		if match == nil {
			t.Fatalf("%s is not declared in capacity.ts", c.constant)
		}
		got, err := strconv.ParseFloat(string(match[1]), 64)
		if err != nil {
			t.Fatalf("parsing %s: %v", c.constant, err)
		}
		if got != c.want {
			t.Fatalf("%s = %v in capacity.ts but %v in core/forecast — the two must not drift", c.constant, got, c.want)
		}
	}
}
