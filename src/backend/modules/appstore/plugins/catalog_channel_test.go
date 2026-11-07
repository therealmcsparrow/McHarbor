// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

const catalogEntryScript = "../../../../../scripts/development/catalog_entry.py"

// pythonForCatalog finds an interpreter the way build-plugin.sh does. A missing one
// fails the test: skipping would let the catalog writer go unchecked on a bare machine.
func pythonForCatalog(t *testing.T) string {
	t.Helper()
	for _, c := range []string{os.Getenv("MCHARBOR_PYTHON"), "python3", "python", "py"} {
		if c == "" {
			continue
		}
		if _, err := exec.LookPath(c); err != nil {
			continue
		}
		if exec.Command(c, "-c", "import json").Run() == nil {
			return c
		}
	}
	t.Fatal("no usable python found: catalog_entry.py cannot be tested")
	return ""
}

func runCatalogEntry(t *testing.T, args ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs(catalogEntryScript)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pythonForCatalog(t), append([]string{"-I", script}, args...)...).CombinedOutput()
	return string(out), err
}

type catalogRun struct {
	dir, version, floor string
}

func (c catalogRun) write(t *testing.T) (string, error) {
	t.Helper()
	manifest := filepath.Join(c.dir, "plugin.yaml")
	if err := os.WriteFile(manifest, []byte("name: \"Demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return runCatalogEntry(t, "write",
		"--src", filepath.Join(c.dir, "src.json"),
		"--out", filepath.Join(c.dir, "demo.json"),
		"--slug", "demo",
		"--version", c.version,
		"--url", "https://example.invalid/files/demo-"+c.version+".tar.gz",
		"--manifest", manifest,
		"--min-mcharbor", c.floor,
		"--signature", "sig-"+c.version,
		"--build", "202611070400")
}

// writtenEntry is the entry file as the v2 contract names it: stable rows under
// versions, prerelease rows under their own key.
type writtenEntry struct {
	Versions           []PluginVersion `json:"versions"`
	PrereleaseVersions []PluginVersion `json:"prerelease_versions"`
}

func readWritten(t *testing.T, path string) writtenEntry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "") {
		t.Error("catalog entry carries CR: the writer must emit LF")
	}
	var e writtenEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func versionNames(vs []PluginVersion) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Version
	}
	return out
}

// wantRows asserts the stable and prerelease lists of the written entry.
func wantRows(t *testing.T, dir string, stable, pre []string) writtenEntry {
	t.Helper()
	got := readWritten(t, filepath.Join(dir, "demo.json"))
	if g, w := strings.Join(versionNames(got.Versions), ","), strings.Join(stable, ","); g != w {
		t.Fatalf("versions = %v, want %v", versionNames(got.Versions), stable)
	}
	if g, w := strings.Join(versionNames(got.PrereleaseVersions), ","), strings.Join(pre, ","); g != w {
		t.Fatalf("prerelease_versions = %v, want %v", versionNames(got.PrereleaseVersions), pre)
	}
	return got
}

// baselineCatalogFile is the file shape the readers on today's release decode:
// no prerelease_versions field, so that key is ignored.
type baselineCatalogFile struct {
	Slug        string          `json:"slug"`
	Version     string          `json:"version"`
	DownloadURL string          `json:"download_url"`
	Versions    []PluginVersion `json:"versions"`
}

// baselineVisible returns the versions a host on today's reader logic lists for
// one entry: versions[] plus the legacy top-level row, nothing else.
func baselineVisible(t *testing.T, raw []byte) []string {
	t.Helper()
	var f baselineCatalogFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, v := range f.Versions {
		if v.Version != "" && v.DownloadURL != "" {
			out = append(out, v.Version)
		}
	}
	if f.Version != "" && f.DownloadURL != "" {
		out = append(out, f.Version)
	}
	return out
}

func requireAllStable(t *testing.T, where string, versions []string) {
	t.Helper()
	for _, v := range versions {
		if versionorder.IsPrerelease(v) {
			t.Errorf("%s: a host on today's reader logic would list prerelease %s", where, v)
		}
	}
}

// TestCatalogKeepsBothKinds walks the sequence the plan names: stable first, the
// newest of each kind kept, an older prerelease dropped, the floor's suffix kept verbatim.
func TestCatalogKeepsBothKinds(t *testing.T) {
	dir := t.TempDir()
	step := func(version, floor string) {
		t.Helper()
		if out, err := (catalogRun{dir, version, floor}).write(t); err != nil {
			t.Fatalf("write %s: %v\n%s", version, err, out)
		}
	}

	step("2026.11.01", "2026.11.01")
	wantRows(t, dir, []string{"2026.11.01"}, nil)

	step("2026.11.02-rc.1", "2026.11.02-rc.1")
	got := wantRows(t, dir, []string{"2026.11.01"}, []string{"2026.11.02-rc.1"})
	if got.PrereleaseVersions[0].MinMcharbor != "2026.11.02-rc.1" {
		t.Errorf("prerelease floor = %q, the suffix must survive", got.PrereleaseVersions[0].MinMcharbor)
	}

	step("2026.11.02-rc.2", "2026.11.01")
	wantRows(t, dir, []string{"2026.11.01"}, []string{"2026.11.02-rc.2"})

	step("2026.11.02-rc.10", "2026.11.01")
	wantRows(t, dir, []string{"2026.11.01"}, []string{"2026.11.02-rc.10"})

	step("2026.11.03", "2026.11.01")
	wantRows(t, dir, []string{"2026.11.03"}, nil)

	step("2026.11.04-rc.1", "2026.11.03")
	wantRows(t, dir, []string{"2026.11.03"}, []string{"2026.11.04-rc.1"})

	step("2026.11.03", "2026.11.01")
	wantRows(t, dir, []string{"2026.11.03"}, []string{"2026.11.04-rc.1"})
}

// TestCatalogOldHostSeesOnlyStable is the compatibility guarantee: whatever floor a
// prerelease declares, versions[] holds stable rows alone, so a host on today's reader
// logic, which sorts versions[] and ignores unknown keys, can never rank a prerelease first.
func TestCatalogOldHostSeesOnlyStable(t *testing.T) {
	dir := t.TempDir()
	for _, step := range []catalogRun{
		{dir, "2026.11.01", "2026.09.01"},
		{dir, "2026.11.02-rc.1", "2026.09.01"},
	} {
		if out, err := step.write(t); err != nil {
			t.Fatalf("write %s: %v\n%s", step.version, err, out)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	seen := baselineVisible(t, raw)
	if strings.Join(seen, ",") != "2026.11.01" {
		t.Fatalf("today's readers list %v, want only 2026.11.01", seen)
	}
	requireAllStable(t, "demo.json", seen)

	onlyPre := t.TempDir()
	if out, err := (catalogRun{onlyPre, "2026.11.02-rc.1", "2026.09.01"}).write(t); err != nil {
		t.Fatalf("prerelease-only write: %v\n%s", err, out)
	}
	rawPre, err := os.ReadFile(filepath.Join(onlyPre, "demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	if seen := baselineVisible(t, rawPre); len(seen) != 0 {
		t.Fatalf("a prerelease-only entry is visible to today's readers: %v", seen)
	}
	wantRows(t, onlyPre, nil, []string{"2026.11.02-rc.1"})
}

// TestCatalogMigratesAPrereleaseInVersions rewrites an entry an earlier writer left
// with a prerelease inside versions[] into the v2 shape.
func TestCatalogMigratesAPrereleaseInVersions(t *testing.T) {
	dir := t.TempDir()
	old := `{"slug":"demo","versions":[
		{"version":"2026.11.01","download_url":"https://example.invalid/a"},
		{"version":"2026.11.02-rc.1","download_url":"https://example.invalid/b"}]}`
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := (catalogRun{dir, "2026.11.02-rc.2", "2026.09.01"}).write(t); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	wantRows(t, dir, []string{"2026.11.01"}, []string{"2026.11.02-rc.2"})
}

// TestCatalogRefusesHostileInput covers values that reach file names and URL paths.
func TestCatalogRefusesHostileInput(t *testing.T) {
	for _, tc := range []catalogRun{
		{version: "2026.11.02-../x", floor: "2026.11.01"},
		{version: "2026.11.02-rc/1", floor: "2026.11.01"},
		{version: "latest", floor: "2026.11.01"},
		{version: "2026.11.02-rc.1", floor: "2026.11.02-"},
		{version: "2026.11.02-rc.1", floor: "2026.11.02-../x"},
	} {
		tc.dir = t.TempDir()
		if out, err := tc.write(t); err == nil {
			t.Errorf("%+v was accepted:\n%s", tc, out)
		}
	}
}

// TestCatalogKeepsFileOnBadMerge shows a corrupt or newer existing entry is never overwritten.
func TestCatalogKeepsFileOnBadMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := (catalogRun{dir, "2026.11.01", "2026.11.01"}).write(t); err == nil {
		t.Fatalf("a corrupt existing entry must stop the write:\n%s", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "{not json" {
		t.Errorf("the corrupt file was replaced (read err: %v)", err)
	}

	dir = t.TempDir()
	if out, err := (catalogRun{dir, "2026.11.05", "2026.11.01"}).write(t); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
	if out, err := (catalogRun{dir, "2026.11.01", "2026.11.01"}).write(t); err == nil {
		t.Fatalf("an older stable than the catalog holds would be dropped silently:\n%s", out)
	}
	wantRows(t, dir, []string{"2026.11.05"}, nil)
}

type versionCases struct {
	Compare []struct {
		A    string `yaml:"a"`
		B    string `yaml:"b"`
		Want int    `yaml:"want"`
	} `yaml:"compare"`
	Parse []struct {
		Input string `yaml:"input"`
		Valid bool   `yaml:"valid"`
	} `yaml:"parse"`
	Prerelease []struct {
		Input string `yaml:"input"`
		Want  bool   `yaml:"want"`
	} `yaml:"prerelease"`
}

// TestCatalogEntryMatchesVersionOrderCases runs every row of the shared contract table
// through the script, so Python cannot order versions differently from Go.
func TestCatalogEntryMatchesVersionOrderCases(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/versionorder/cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var tbl versionCases
	if err := yaml.Unmarshal(raw, &tbl); err != nil {
		t.Fatal(err)
	}
	if len(tbl.Compare) == 0 || len(tbl.Parse) == 0 || len(tbl.Prerelease) == 0 {
		t.Fatal("case table has an empty section")
	}
	for _, c := range tbl.Compare {
		out, err := runCatalogEntry(t, "--compare", c.A, c.B)
		if err != nil || strings.TrimSpace(out) != strconv.Itoa(c.Want) {
			t.Errorf("--compare %q %q = %q (%v), want %d", c.A, c.B, out, err, c.Want)
		}
	}
	for _, c := range tbl.Parse {
		out, err := runCatalogEntry(t, "--parse", c.Input)
		if err != nil {
			t.Fatalf("--parse %q: %v %s", c.Input, err, out)
		}
		if strings.TrimSpace(out) != strconv.FormatBool(c.Valid) {
			t.Errorf("--parse %q = %q, want %v", c.Input, out, c.Valid)
		}
	}
	for _, c := range tbl.Prerelease {
		out, err := runCatalogEntry(t, "--prerelease", c.Input)
		if err != nil {
			t.Fatalf("--prerelease %q: %v %s", c.Input, err, out)
		}
		if strings.TrimSpace(out) != strconv.FormatBool(c.Want) {
			t.Errorf("--prerelease %q = %q, want %v", c.Input, out, c.Want)
		}
	}
}

func TestReadersOrderStableFirst(t *testing.T) {
	dir := t.TempDir()
	body := `{"slug":"demo","name":"Demo","versions":[
		{"version":"2026.11.02-rc.2","download_url":"https://example.invalid/a"},
		{"version":"2026.11.01","download_url":"https://example.invalid/b"},
		{"version":"2026.11.02-rc.10","download_url":"https://example.invalid/c"}]}`
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := readLocalCatalogDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read: %v %d", err, len(entries))
	}
	e := entries[0]
	if got := strings.Join(versionNames(e.Versions), ","); got != "2026.11.01,2026.11.02-rc.10,2026.11.02-rc.2" {
		t.Errorf("order = %s", got)
	}
	if e.Version != "2026.11.01" || e.DownloadURL != "https://example.invalid/b" {
		t.Errorf("top-level mirror = %s %s, want the newest stable", e.Version, e.DownloadURL)
	}
}

func TestRemoteCatalogRefusesUnparsableVersionRows(t *testing.T) {
	entry, ok := entryFromCatalogFile(catalogFile{Slug: "demo", Versions: []PluginVersion{
		{Version: "latest", DownloadURL: "https://example.invalid/a"},
		{Version: "2026.11.02-../x", DownloadURL: "https://example.invalid/b"},
		{Version: "2026.11.01", DownloadURL: "https://example.invalid/c"},
	}})
	if !ok || len(entry.Versions) != 1 || entry.Versions[0].Version != "2026.11.01" {
		t.Fatalf("got %+v ok=%v, want only the parsable row", entry.Versions, ok)
	}
	if _, ok := entryFromCatalogFile(catalogFile{Slug: "demo", Versions: []PluginVersion{
		{Version: "latest", DownloadURL: "https://example.invalid/a"},
	}}); ok {
		t.Error("an entry with no parsable version must be refused")
	}
}

// TestReadersTakePrereleaseVersions reads the v2 key: the prerelease rows join the
// version list behind the stable ones, and an entry holding only prereleases is kept
// so a plugin that opted in can be offered it.
func TestReadersTakePrereleaseVersions(t *testing.T) {
	body := `{"slug":"demo","name":"Demo",
		"versions":[{"version":"2026.11.01","download_url":"https://example.invalid/b"}],
		"prerelease_versions":[
			{"version":"2026.11.02-rc.2","download_url":"https://example.invalid/a"},
			{"version":"2026.11.03","download_url":"https://example.invalid/smuggled-stable"},
			{"version":"2026.11.02-../x","download_url":"https://example.invalid/bad"}]}`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	local, err := readLocalCatalogDir(dir)
	if err != nil || len(local) != 1 {
		t.Fatalf("read: %v %d", err, len(local))
	}
	var cf catalogFile
	if err := json.Unmarshal([]byte(body), &cf); err != nil {
		t.Fatal(err)
	}
	remote, ok := entryFromCatalogFile(cf)
	if !ok {
		t.Fatal("remote reader refused the entry")
	}
	for name, e := range map[string]PluginEntry{"local": local[0], "remote": remote} {
		if got := strings.Join(versionNames(e.Versions), ","); got != "2026.11.01,2026.11.02-rc.2" {
			t.Errorf("%s: versions = %s, want the stable then the prerelease, with no stable row smuggled in", name, got)
		}
		if e.Version != "2026.11.01" {
			t.Errorf("%s: top-level mirror = %s, want the newest stable", name, e.Version)
		}
	}

	onlyPre := `{"slug":"solo","versions":[],"prerelease_versions":[{"version":"2026.11.02-rc.1","download_url":"https://example.invalid/p"}]}`
	if err := os.WriteFile(filepath.Join(dir, "solo.json"), []byte(onlyPre), 0o600); err != nil {
		t.Fatal(err)
	}
	local, err = readLocalCatalogDir(dir)
	if err != nil || len(local) != 2 {
		t.Fatalf("a prerelease-only file was dropped: %v %d", err, len(local))
	}
	var cfPre catalogFile
	if err := json.Unmarshal([]byte(onlyPre), &cfPre); err != nil {
		t.Fatal(err)
	}
	if _, ok := entryFromCatalogFile(cfPre); !ok {
		t.Error("remote reader dropped a prerelease-only entry")
	}
}

// TestIndexBuilderKeepsTheV2Split runs build-plugin-catalog-index.sh over a folder of
// entries and checks index.json: stable rows under versions, prereleases under their
// own key, so the index is as safe for today's readers as the entries.
func TestIndexBuilderKeepsTheV2Split(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is needed to run the index builder: %v", err)
	}
	dir := t.TempDir()
	entries := map[string]string{
		"demo.json": `{"slug":"demo","name":"Demo",
			"versions":[{"version":"2026.11.01","download_url":"https://example.invalid/b"},
				{"version":"2026.10.09","download_url":"https://example.invalid/c"}],
			"prerelease_versions":[
				{"version":"2026.11.02-rc.2","download_url":"https://example.invalid/a2"},
				{"version":"2026.11.02-rc.10","download_url":"https://example.invalid/a10"}]}`,
		"solo.json": `{"slug":"solo","name":"Solo","versions":[],
			"prerelease_versions":[{"version":"2026.11.02-rc.1","download_url":"https://example.invalid/p"}]}`,
	}
	for name, body := range entries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script, err := filepath.Abs("../../../../../scripts/development/build-plugin-catalog-index.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, script)
	cmd.Env = append(os.Environ(), "MCHARBOR_PLUGIN_OUTPUT_DIR="+dir, "MCHARBOR_PYTHON="+pythonForCatalog(t))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("index builder: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index []struct {
		Slug string `json:"slug"`
		writtenEntry
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if len(index) != 2 {
		t.Fatalf("index holds %d entries, want demo and solo: %s", len(index), raw)
	}
	for _, e := range index {
		switch e.Slug {
		case "demo":
			if got := strings.Join(versionNames(e.Versions), ","); got != "2026.11.01,2026.10.09" {
				t.Errorf("demo versions = %s, want the stable rows newest first", got)
			}
			if got := strings.Join(versionNames(e.PrereleaseVersions), ","); got != "2026.11.02-rc.10,2026.11.02-rc.2" {
				t.Errorf("demo prerelease_versions = %s, want rc.10 before rc.2", got)
			}
		case "solo":
			if len(e.Versions) != 0 || len(e.PrereleaseVersions) != 1 {
				t.Errorf("solo = %+v, want an empty versions list and one prerelease", e)
			}
		}
	}
	var asEntries []json.RawMessage
	if err := json.Unmarshal(raw, &asEntries); err != nil {
		t.Fatal(err)
	}
	for _, one := range asEntries {
		requireAllStable(t, "index.json", baselineVisible(t, one))
	}
}
