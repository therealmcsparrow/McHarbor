// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
)

// buildMountTar constructs an in-memory mount tar with the given (path, size, typeflag) entries.
func buildMountTar(t *testing.T, entries []struct {
	name    string
	size    int64
	isDir   bool
	content string
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o644,
			Size:     e.size,
			Typeflag: tar.TypeReg,
		}
		if e.isDir {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if !e.isDir && e.content != "" {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatalf("write content: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// buildOuterArchive constructs an outer tar with manifest.json + image + filesystem + optional
// mount tar entries. mountEntries maps mount target -> (path, size, isDir, content) tuples.
func buildOuterArchive(t *testing.T, mountEntries map[string][]struct {
	name    string
	size    int64
	isDir   bool
	content string
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// manifest.json — production reads SelectedMounts as []string, not a map.
	mounts := []string{}
	for target := range mountEntries {
		mounts = append(mounts, target)
	}
	sort.Strings(mounts) // deterministic order for tests
	manifest := map[string]any{
		"format": "mcharbor.container.backup.v1",
		"plan": map[string]any{
			"selectedMounts": mounts,
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	mh := &tar.Header{Name: "manifest.json", Size: int64(len(manifestBytes)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(mh); err != nil {
		t.Fatalf("write manifest header: %v", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		t.Fatalf("write manifest body: %v", err)
	}

	// image/image.tar
	ih := &tar.Header{Name: "image/image.tar", Size: 0, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(ih); err != nil {
		t.Fatalf("write image header: %v", err)
	}

	// container/filesystem.tar
	fh := &tar.Header{Name: "container/filesystem.tar", Size: 0, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(fh); err != nil {
		t.Fatalf("write filesystem header: %v", err)
	}

	// mount entries
	for target, entries := range mountEntries {
		// Mirror the slug rules from backupMountEntryName.
		slug := safeArchiveName.ReplaceAllString(strings.Trim(target, "/"), "-")
		mountBytes := buildMountTar(t, entries)
		mh := &tar.Header{Name: "mounts/" + slug + ".tar", Size: int64(len(mountBytes)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(mh); err != nil {
			t.Fatalf("write mount header: %v", err)
		}
		if _, err := tw.Write(mountBytes); err != nil {
			t.Fatalf("write mount body: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close outer: %v", err)
	}
	return buf.Bytes()
}

func TestRestoreArchiveOptions_ImageOnly(t *testing.T) {
	outer := buildOuterArchive(t, nil)
	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}
	want := []string{"image", "filesystem"}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	for i, k := range want {
		if items[i].Key != k {
			t.Errorf("item %d key = %q, want %q", i, items[i].Key, k)
		}
	}
}

func TestRestoreArchiveOptions_MountWithFiles(t *testing.T) {
	entries := []struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		{name: "data", isDir: true},
		{name: "data/file.txt", size: 12, content: "hello world\n"},
		{name: "data/sub", isDir: true},
		{name: "data/sub/inner.txt", size: 5, content: "inner"},
	}
	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{"/data": entries})

	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}

	var mountRow *BackupOption
	children := []BackupOption{}
	for i := range items {
		if items[i].Type == "mount" {
			mountRow = &items[i]
		} else {
			children = append(children, items[i])
		}
	}
	if mountRow == nil {
		t.Fatalf("missing mount row: %+v", items)
	}
	if mountRow.Key != "mount:/data" {
		t.Errorf("mount key = %q, want %q", mountRow.Key, "mount:/data")
	}
	if mountRow.Truncated {
		t.Errorf("mount Truncated should be false for small entry")
	}

	if len(children) < 4 {
		t.Fatalf("expected at least 4 children, got %d: %+v", len(children), children)
	}

	// The mount root folder (path "data") should appear somewhere in the
	// children list — its position depends on outer-tar ordering.
	rootFound := false
	var fileCount, folderCount int
	for _, c := range children {
		switch c.Type {
		case "folder":
			folderCount++
			if c.Path == "data" {
				rootFound = true
			}
		case "file":
			fileCount++
			if c.ParentMount != "/data" {
				t.Errorf("file parentMount = %q, want /data", c.ParentMount)
			}
		}
	}
	if !rootFound {
		t.Errorf("expected mount-root folder path=data, got children=%+v", children)
	}
	if fileCount < 1 {
		t.Errorf("expected at least one file child, got %d", fileCount)
	}
	if folderCount < 2 {
		t.Errorf("expected at least two folder children (root + sub), got %d", folderCount)
	}
}

func TestRestoreArchiveOptions_MultiSegmentMounts(t *testing.T) {
	// Simulates a Nextcloud backup with two multi-segment mounts.
	// The frontend sends the original mount path (e.g. `/var/www/html/data`)
	// to the lazy-load endpoint; the backend must resolve that against
	// the slug-encoded archive entry name `mounts/var-www-html-data.tar`.
	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		"/var/www/html/data": {
			{name: "data", isDir: true},
			{name: "data/config.php", size: 1, content: "x"},
		},
		"/var/www/html": {
			{name: "html", isDir: true},
			{name: "html/index.php", size: 1, content: "y"},
		},
	})

	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}

	mountKeys := map[string]bool{}
	for _, it := range items {
		if it.Type == "mount" {
			mountKeys[it.Key] = true
		}
	}
	for _, want := range []string{"mount:/var/www/html/data", "mount:/var/www/html"} {
		if !mountKeys[want] {
			t.Errorf("missing mount row %q in items", want)
		}
	}
}

func TestMountEntriesForRestore_MultiSegment(t *testing.T) {
	// Reproduces the user's "Could not load files for this mount" bug
	// for a Nextcloud backup with a multi-segment mount target. The
	// lazy-load endpoint receives `/var/www/html/data` (URL-decoded) and
	// must resolve it to `mounts/var-www-html-data.tar` via the manifest.
	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		"/var/www/html/data": {
			{name: "data", isDir: true},
			{name: "data/config.php", size: 5, content: "<?php"},
			{name: "data/themes", isDir: true},
		},
	})

	tr := tar.NewReader(bytes.NewReader(outer))
	var innerBytes []byte
	for {
		hdr, err := tr.Next()
		if err != nil {
			t.Fatalf("read outer: %v", err)
		}
		if hdr.Name == "mounts/var-www-html-data.tar" {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				t.Fatalf("copy inner: %v", err)
			}
			innerBytes = buf.Bytes()
			break
		}
	}
	entries, _, err := enumerateMountChildrenAt(tar.NewReader(bytes.NewReader(innerBytes)), "/var/www/html/data", "")
	if err != nil {
		t.Fatalf("enumerateMountChildrenAt: %v", err)
	}
	// Direct children of the root (innerPath="") are the top-level
	// entries of the mount tar. The lazy-load endpoint surfaces a
	// folder row for `data`; the file `data/config.php` lives one
	// segment deeper and is reached by expanding `data` next.
	var sawDataFolder bool
	for _, e := range entries {
		if e.Type == "folder" && e.Path == "data" {
			sawDataFolder = true
		}
	}
	if !sawDataFolder {
		t.Errorf("expected folder child data, got %+v", entries)
	}

	// Now drill into `data` and verify the config.php file appears.
	entries2, _, err := enumerateMountChildrenAt(tar.NewReader(bytes.NewReader(innerBytes)), "/var/www/html/data", "data")
	if err != nil {
		t.Fatalf("enumerateMountChildrenAt nested: %v", err)
	}
	var sawConfig bool
	for _, e := range entries2 {
		if e.Type == "file" && e.Path == "data/config.php" {
			sawConfig = true
		}
	}
	if !sawConfig {
		t.Errorf("expected file child data/config.php after drilling into data, got %+v", entries2)
	}
}

func TestRestoreArchiveOptions_TruncatesLargeMount(t *testing.T) {
	entries := make([]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}, 0, restoreOptionsMaxMountEntries+500)
	// One root dir, then 1500 files.
	entries = append(entries, struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{name: "data", isDir: true})
	for i := 0; i < restoreOptionsMaxMountEntries+500; i++ {
		entries = append(entries, struct {
			name    string
			size    int64
			isDir   bool
			content string
		}{name: "data/file_" + itoa(i) + ".txt", size: 4, content: "test"})
	}

	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{"/data": entries})

	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}

	var mountRow *BackupOption
	childCount := 0
	for i := range items {
		if items[i].Type == "mount" {
			mountRow = &items[i]
		} else {
			childCount++
		}
	}
	if mountRow == nil {
		t.Fatalf("missing mount row")
	}
	if !mountRow.Truncated {
		t.Errorf("expected Truncated=true on mount row")
	}
	// children count = image + filesystem + mount-root folder (path="data") + 999 files
	// capped at restoreOptionsMaxMountEntries (1000) of mount descendants.
	wantChildren := 2 /*image+filesystem*/ + restoreOptionsMaxMountEntries
	if childCount != wantChildren {
		t.Errorf("expected %d children, got %d", wantChildren, childCount)
	}
}

// TestMountLookup_MultiSegmentURLDecoded simulates the wire flow the
// Nextcloud operator hit: the frontend URL-encodes the mount target
// (`/var/www/html/data` → `%2Fvar%2Fwww%2Fhtml%2Fdata`), chi keeps it
// encoded, and the handler must URL-decode before matching against
// manifest.json selectedMounts. Replays the matching loop from
// MountEntriesForRestore minus the DB + decryption.
func TestMountLookup_MultiSegmentURLDecoded(t *testing.T) {
	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		"/var/www/html/data": {
			{name: "data", isDir: true},
			{name: "data/config.php", size: 5, content: "<?php"},
		},
		"/var/www/html": {
			{name: "html", isDir: true},
		},
	})

	// Same URL the frontend would build via encodeURIComponent.
	wireMountTarget := "%2Fvar%2Fwww%2Fhtml%2Fdata"
	decoded := decodeRestoreMountTarget(wireMountTarget)
	if decoded != "/var/www/html/data" {
		t.Fatalf("decode helper returned %q, want %q", decoded, "/var/www/html/data")
	}

	tr := tar.NewReader(bytes.NewReader(outer))
	mountTargets := map[string]string{}
	var foundTarget string
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "manifest.json" {
			mountTargets = restoreMountTargets(tr)
			continue
		}
		if strings.HasPrefix(hdr.Name, "mounts/") && strings.HasSuffix(hdr.Name, ".tar") {
			target := mountTargets[hdr.Name]
			if target == "" {
				target = strings.TrimSuffix(strings.TrimPrefix(hdr.Name, "mounts/"), ".tar")
			}
			if target == decoded {
				foundTarget = target
			}
		}
	}
	if foundTarget != "/var/www/html/data" {
		t.Fatalf("URL-decoded mount target %q not matched against manifest entries %+v", decoded, mountTargets)
	}
}

func TestRestoreArchiveOptions_FolderKeyEncoding(t *testing.T) {
	entries := []struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		{name: "data", isDir: true},
		{name: "data/file.txt", size: 3, content: "abc"},
	}
	outer := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{"/data": entries})

	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}

	folderFound := false
	fileFound := false
	for _, it := range items {
		switch it.Type {
		case "folder":
			folderFound = true
			if !strings.HasSuffix(it.Key, "/") {
				t.Errorf("folder key %q must end with /", it.Key)
			}
			if it.Path == "" {
				t.Errorf("folder path empty")
			}
		case "file":
			fileFound = true
			if strings.HasSuffix(it.Key, "/") {
				t.Errorf("file key %q must not end with /", it.Key)
			}
			if it.Size == 0 {
				t.Errorf("file size is 0")
			}
		}
	}
	if !folderFound || !fileFound {
		t.Fatalf("expected both folder and file rows, got %+v", items)
	}
}

func TestPrecomputeWantedFilesByMount(t *testing.T) {
	items := []string{
		"image",
		"filesystem",
		"mount:/data",
		"file:/data:data/file.txt",
		"file:/data:data/sub/inner.txt",
		"file:/data:data/sub/", // folder
		"file:/logs:logs.txt",
	}
	files, folders := precomputeWantedFilesByMount(items)

	if !files["/data"]["data/file.txt"] {
		t.Errorf("expected /data:data/file.txt in files")
	}
	if !files["/data"]["data/sub/inner.txt"] {
		t.Errorf("expected /data:data/sub/inner.txt in files")
	}
	if !folders["/data"]["data/sub"] {
		t.Errorf("expected /data:data/sub in folders")
	}
	if !files["/logs"]["logs.txt"] {
		t.Errorf("expected /logs:logs.txt in files")
	}
	if _, ok := files["/missing"]; ok {
		t.Errorf("did not expect /missing key")
	}
}

func TestInnerPathMatchesWanted(t *testing.T) {
	files := map[string]bool{"data/file.txt": true}
	folders := map[string]bool{"data/sub": true}

	cases := []struct {
		path string
		want bool
	}{
		{"data/file.txt", true},
		{"data/sub", true},
		{"data/sub/inner.txt", true},
		{"data/other.txt", false},
		{"other/file.txt", false},
	}
	for _, c := range cases {
		if got := innerPathMatchesWanted(c.path, files, folders); got != c.want {
			t.Errorf("innerPathMatchesWanted(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestFilterMountTarEntries(t *testing.T) {
	innerBytes := buildMountTar(t, []struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{
		{name: "data", isDir: true},
		{name: "data/a.txt", size: 2, content: "aa"},
		{name: "data/sub", isDir: true},
		{name: "data/sub/b.txt", size: 2, content: "bb"},
		{name: "data/c.txt", size: 2, content: "cc"},
	})
	tr := tar.NewReader(bytes.NewReader(innerBytes))
	files := map[string]bool{"data/a.txt": true}
	folders := map[string]bool{"data/sub": true}
	buf, totalSize, err := filterMountTarEntries(tr, files, folders)
	if err != nil {
		t.Fatalf("filterMountTarEntries: %v", err)
	}
	if totalSize == 0 {
		t.Errorf("expected non-zero totalSize")
	}
	// Verify the rebuilt tar contains the expected entries.
	reread := tar.NewReader(buf)
	got := []string{}
	for {
		hdr, err := reread.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("re-read: %v", err)
		}
		got = append(got, hdr.Name)
	}
	wantExact := map[string]bool{
		"data":           true,
		"data/a.txt":     true,
		"data/sub":       true,
		"data/sub/b.txt": true,
	}
	if len(got) != len(wantExact) {
		t.Errorf("got %d entries, want %d: %v", len(got), len(wantExact), got)
	}
	for _, name := range got {
		if !wantExact[name] {
			t.Errorf("unexpected entry %q in filtered tar", name)
		}
	}
}

// itoa is a tiny strconv-free formatter for test names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// buildOuterArchiveWithInspect builds an outer tar that includes a
// synthetic `container/inspect.json` payload so the per-item config
// tree emission path can be exercised end-to-end.
func buildOuterArchiveWithInspect(t *testing.T, env []string, cmd []string, labels map[string]string, workingDir, user, image string) []byte {
	t.Helper()
	type cfg struct {
		Config struct {
			Env        []string          `json:"Env"`
			Cmd        []string          `json:"Cmd"`
			Entrypoint []string          `json:"Entrypoint"`
			Labels     map[string]string `json:"Labels"`
			WorkingDir string            `json:"WorkingDir"`
			User       string            `json:"User"`
			Image      string            `json:"Image"`
		} `json:"Config"`
	}
	var c cfg
	c.Config.Env = env
	c.Config.Cmd = cmd
	c.Config.Labels = labels
	c.Config.WorkingDir = workingDir
	c.Config.User = user
	c.Config.Image = image
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal inspect: %v", err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "container/inspect.json", Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write inspect header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write inspect body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// TestRestoreArchiveOptions_ConfigTree verifies that an archive
// containing `container/inspect.json` causes `restoreArchiveOptions`
// to emit a synthetic `config` root row plus per-field folder rows
// the per-item picker UI drills into. Without this emission the
// picker never appears in the dialog — the operator only sees the
// binary toggle.
func TestRestoreArchiveOptions_ConfigTree(t *testing.T) {
	outer := buildOuterArchiveWithInspect(t,
		[]string{"FOO=1", "BAR=2"},
		[]string{"nginx", "-g", "daemon off;"},
		map[string]string{"com.example.team": "ops"},
		"/app",
		"nginx",
		"nginx:1.27",
	)

	// First, sanity check the archive itself.
	{
		tr := tar.NewReader(bytes.NewReader(outer))
		for {
			hdr, err := tr.Next()
			if err != nil {
				break
			}
			if hdr.Name != "container/inspect.json" {
				t.Errorf("archive has unexpected entry %q", hdr.Name)
			}
		}
	}

	items, err := restoreArchiveOptions(bytes.NewReader(outer))
	if err != nil {
		t.Fatalf("restoreArchiveOptions: %v", err)
	}

	var configRoot *BackupOption
	folderByKey := map[string]*BackupOption{}
	for i := range items {
		if items[i].Type == "config" {
			configRoot = &items[i]
		}
		if items[i].Type == "folder" && items[i].ParentMount == "config" {
			folderByKey[items[i].Key] = &items[i]
		}
	}
	if configRoot == nil {
		t.Fatalf("missing synthetic config root row; items=%+v", items)
	}
	if configRoot.Key != "config" {
		t.Errorf("config root key = %q, want %q", configRoot.Key, "config")
	}
	if configRoot.EntryKind != "folder" {
		t.Errorf("config root EntryKind = %q, want folder", configRoot.EntryKind)
	}
	for _, want := range []string{
		"config:Config.Env",
		"config:Config.Cmd",
		"config:Config.Entrypoint",
		"config:Config.Args",
		"config:Config.Labels",
		"config:Config.ExposedPorts",
		"config:Config.Volumes",
	} {
		if _, ok := folderByKey[want]; !ok {
			t.Errorf("missing config folder row %q in items", want)
		}
	}
	for _, want := range []string{
		"config:Config.WorkingDir",
		"config:Config.User",
		"config:Config.Image",
	} {
		found := false
		for _, it := range items {
			if it.Key == want && it.Type == "file" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing scalar config leaf %q in items", want)
		}
	}
}
