// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
)

// TestLoaderRefusesAReservedSlug: a plugin already on disk under the slug "host" (installed
// before the reservation) is never loaded, so it cannot act as the host's fire source.
func TestLoaderRefusesAReservedSlug(t *testing.T) {
	dir := t.TempDir()
	write := func(slug string) string {
		path := filepath.Join(dir, slug+".yaml")
		body := "schema: \"mcharbor.plugin/v1\"\nslug: \"" + slug + "\"\nname: \"Test\"\nversion: \"2026.09.01\"\nauthor: \"McSparrow\"\nmcharbor: \">=2026.09.01\"\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if _, err := readManifestFile(write("host")); !errors.Is(err, pluginslug.ErrReserved) {
		t.Fatalf("a manifest slugged host must be refused as reserved, got %v", err)
	}
	if _, err := readManifestFile(write("addon-demo")); err != nil {
		t.Fatalf("an ordinary manifest must still load: %v", err)
	}
}
