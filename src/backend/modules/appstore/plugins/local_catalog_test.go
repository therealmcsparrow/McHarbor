// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRebaseLocalFileURL(t *testing.T) {
	t.Run("original path exists returns url unchanged", func(t *testing.T) {
		origDir := t.TempDir()
		targetDir := t.TempDir()
		origPath := filepath.Join(origDir, "y.tar.gz")
		if err := os.WriteFile(origPath, []byte("y"), 0o644); err != nil {
			t.Fatal(err)
		}
		rawURL := "file://" + filepath.ToSlash(origPath)
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != rawURL {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want unchanged %q", targetDir, rawURL, got, rawURL)
		}
	})

	t.Run("rebases when original missing and candidate exists", func(t *testing.T) {
		targetDir := t.TempDir()
		candidatePath := filepath.Join(targetDir, "x.tar.gz")
		if err := os.WriteFile(candidatePath, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		rawURL := "file:///app/ai-plugin-catalog/x.tar.gz"
		want := "file://" + filepath.ToSlash(candidatePath)
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != want {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want %q", targetDir, rawURL, got, want)
		}
	})

	t.Run("neither original nor candidate exists returns url unchanged", func(t *testing.T) {
		targetDir := t.TempDir()
		rawURL := "file:///app/ai-plugin-catalog/z.tar.gz"
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != rawURL {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want unchanged %q", targetDir, rawURL, got, rawURL)
		}
	})

	t.Run("dotdot suffix returns the original untouched", func(t *testing.T) {
		targetDir := t.TempDir()
		rawURL := "file:///app/ai-plugin-catalog/.."
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != rawURL {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want unchanged %q", targetDir, rawURL, got, rawURL)
		}
	})

	t.Run("non-file scheme passes through", func(t *testing.T) {
		targetDir := t.TempDir()
		rawURL := "https://example.com/x.tar.gz"
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != rawURL {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want unchanged %q", targetDir, rawURL, got, rawURL)
		}
	})

	t.Run("unparseable url passes through", func(t *testing.T) {
		targetDir := t.TempDir()
		rawURL := "://bad"
		got := rebaseLocalFileURL(targetDir, rawURL)
		if got != rawURL {
			t.Errorf("rebaseLocalFileURL(%q, %q) = %q, want unchanged %q", targetDir, rawURL, got, rawURL)
		}
	})
}
