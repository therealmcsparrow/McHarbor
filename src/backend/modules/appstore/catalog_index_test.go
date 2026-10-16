// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogDirectorySkipsPublishedIndex keeps a directory catalog loadable when it also holds
// the generated index.json, which carries no slug of its own and would fail the whole load.
func TestCatalogDirectorySkipsPublishedIndex(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("alpha.json", `{"slug":"alpha","name":"Alpha","image":"alpha:latest"}`)
	write("index.json", `{"version":"1.0.0","apps":[{"slug":"alpha","name":"Alpha","image":"alpha:latest"}]}`)

	catalog, err := loadCatalogDirectory(dir)
	if err != nil {
		t.Fatalf("directory with an index.json did not load: %v", err)
	}
	if len(catalog.Apps) != 1 || catalog.Apps[0].Slug != "alpha" {
		t.Errorf("apps = %+v, want only alpha", catalog.Apps)
	}

	body, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	apps, err := parseRemoteCatalog(body)
	if err != nil || len(apps) != 1 {
		t.Errorf("the index itself did not parse as a remote catalog: %v (%d apps)", err, len(apps))
	}
}
