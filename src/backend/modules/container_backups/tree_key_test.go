// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestTreeWireKeyNamesTheMountByItsTarget(t *testing.T) {
	for _, tc := range []struct{ outer, inner, kind, want string }{
		{"mounts/data", "html/", "folder", "folder:/data:html/"},
		{"mounts/data.tar", "html/index.php", "file", "file:/data:html/index.php"},
		{"mounts/var/lib/app", "cfg", "folder", "folder:/var/lib/app:cfg"},
		{"/data", "html", "folder", "folder:/data:html"},
		{"container/filesystem.tar", "etc/", "folder", "folder:filesystem:etc/"},
	} {
		if got := buildTreeWireKey(tc.outer, tc.inner, tc.kind); got != tc.want {
			t.Errorf("buildTreeWireKey(%q, %q) = %q, want %q", tc.outer, tc.inner, got, tc.want)
		}
	}
}

// TestRestoringATickedFolderRestoresWhatIsInIt is the reported failure: a folder
// ticked inside a mount came back empty, because its key named no mount at all.
func TestRestoringATickedFolderRestoresWhatIsInIt(t *testing.T) {
	// The key exactly as the mount listing emits it, sent the way the picker
	// sends a folder: without the trailing slash.
	sent := strings.TrimSuffix(buildTreeWireKey("mounts/data", "html/", "folder"), "/")
	plan := newRestorePlan([]string{sent})
	unit := runItemMountPfx + "/data"
	if !plan.wantsUnit(unit) || plan.isWhole(unit) {
		t.Fatalf("plan for %q: wants %s = %v, whole = %v; the folder must select part of that mount", sent, unit, plan.wantsUnit(unit), plan.isWhole(unit))
	}
	paths := plan.subsetPaths(unit)
	if !slices.Equal(paths, []string{"html"}) {
		t.Fatalf("subset paths = %v, want [html]", paths)
	}

	mountTar := buildMountTar(t, []struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		{name: "html", isDir: true},
		{name: "html/index.php", size: 2, content: "ok"},
		{name: "html/sub", isDir: true},
		{name: "html/sub/a.txt", size: 1, content: "a"},
		{name: "other.txt", size: 1, content: "o"},
	})
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	if _, err := filterTarSubset(tw, tar.NewReader(bytes.NewReader(mountTar)), paths); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var names []string
	tr := tar.NewReader(&out)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, normalizeTarPath(h.Name))
	}
	for _, want := range []string{"html", "html/index.php", "html/sub", "html/sub/a.txt"} {
		if !slices.Contains(names, want) {
			t.Errorf("restored %v; %q is inside the ticked folder and is missing", names, want)
		}
	}
	if slices.Contains(names, "other.txt") {
		t.Errorf("restored %v; other.txt is outside the ticked folder", names)
	}
}
