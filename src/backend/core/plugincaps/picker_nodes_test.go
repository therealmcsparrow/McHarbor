// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var reNodeSource = regexp.MustCompile(`"source":\s*map\[string\]any\{"plugin":\s*"([^"]+)",\s*"target":\s*"([^"]+)"\}`)

// TestEveryShippedNodePickerNamesADeclaredPickerSource: the loader only logs a plugin-select
// field whose source the manifest does not declare, and the canvas then shows it disabled. This
// turns that silent degradation into a failed test at the source of the node descriptor.
func TestEveryShippedNodePickerNamesADeclaredPickerSource(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]int{}
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(root, d.Name(), "plugin.yaml"))
		if err != nil {
			continue
		}
		caps, err := Parse(d.Name(), raw)
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
		files, err := filepath.Glob(filepath.Join(root, d.Name(), "backend", "*.go"))
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range reNodeSource.FindAllStringSubmatch(string(src), -1) {
				if err := CheckFieldSource(d.Name(), caps, FieldSource{Plugin: m[1], Target: m[2]}); err != nil {
					t.Errorf("%s: %v", filepath.Base(f), err)
				}
				checked[d.Name()]++
			}
		}
	}
	for _, slug := range []string{"addon-kvm", "addon-lxc", "addon-swarm", "addon-kubernetes"} {
		if checked[slug] == 0 {
			t.Errorf("%s declares no plugin-select node field, so nothing here guards its pickers", slug)
		}
	}
}
