// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestInstallTakesTheCatalogBuild guards the store's update badge: an install from a
// catalog URL must record that version's build, or the store offers an "update" to the
// artifact that was just installed.
func TestInstallTakesTheCatalogBuild(t *testing.T) {
	dir := t.TempDir()
	url := "file://" + filepath.ToSlash(filepath.Join(dir, "files", "comm-x-1.0.0.tar.gz"))
	entry := `{"slug":"comm-x","name":"X","versions":[{"version":"1.0.0","build":"202609262330","built_for":"2026.09.01","signature":"c2ln","download_url":"` + url + `"}]}`
	if err := os.WriteFile(filepath.Join(dir, "comm-x.json"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Handler{svc: NewService(nil, nil, dir)}
	got := h.withCatalogStamp(context.Background(), Source{URL: url})
	if got.Build != "202609262330" || got.BuiltFor != "2026.09.01" || got.Signature != "c2ln" {
		t.Fatalf("source = %+v", got)
	}
	if other := h.withCatalogStamp(context.Background(), Source{URL: "file:///elsewhere.tar.gz"}); other.Build != "" {
		t.Fatalf("an URL outside the catalog must stay unstamped: %+v", other)
	}
}
