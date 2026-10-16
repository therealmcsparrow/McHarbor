// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const indexedEntry = `{"slug":"comm-ntfy","name":"ntfy","versions":[{"version":"1.0.0","download_url":"file:///mnt/plugin-catalog/files/comm-ntfy-1.0.0.tar.gz"}]}`

func TestLocalCatalogSkipsTheIndexWithoutAWarning(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"comm-ntfy.json": indexedEntry,
		"index.json":     "[" + indexedEntry + "]",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	entries, err := readLocalCatalogDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Slug != "comm-ntfy" || len(entries[0].Versions) != 1 {
		t.Fatalf("entries = %+v, want comm-ntfy once with one version", entries)
	}
	if strings.Contains(logs.String(), "index.json") {
		t.Fatalf("the index was read as an entry:\n%s", logs.String())
	}
}

func TestLocalCatalogStillWarnsOnABrokenEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("[not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if _, err := readLocalCatalogDir(dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "parse file failed") || !strings.Contains(logs.String(), "broken.json") {
		t.Fatalf("a malformed entry was skipped silently:\n%s", logs.String())
	}
}
