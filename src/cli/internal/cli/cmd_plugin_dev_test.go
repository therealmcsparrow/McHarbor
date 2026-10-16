// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestPluginDevValidateExitsOneOnAnError: the command's whole value in CI is the
// exit code, and a report that always exits 0 is a report nobody gates on.
func TestPluginDevValidateExitsOneOnAnError(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Split(string(body), "contributes:")[0] + "contributes:\n  routes: []\n"
	if err := os.WriteFile(manifest, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := run(t, nil, "", "plugin", "dev", "validate", "--repo", repo, "comm-fixture")
	if code != ExitFailure {
		t.Fatalf("an error finding must exit %d, got %d (%q %q)", ExitFailure, code, out, stderr)
	}
	if !strings.Contains(out, "/api/comm-fixture/send") {
		t.Fatalf("the table must name the route: %q", out)
	}

	// The clean fixture exits 0, so the code above is the finding and not the setup.
	clean := pluginFixture(t, "comm-fixture")
	if code, _, stderr := run(t, nil, "", "plugin", "dev", "validate", "--repo", clean, "comm-fixture"); code != ExitOK {
		t.Fatalf("a clean plugin must exit 0, got %d (%q)", code, stderr)
	}
}

// TestPluginDevNeedsNoServer: the authoring half is local, so it must work with
// no context, no key and nothing listening. A command that reached the API here
// would fail with an auth error instead.
func TestPluginDevNeedsNoServer(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	code, _, stderr := run(t, nil, "", "plugin", "dev", "validate", "--repo", repo, "comm-fixture")
	if code != ExitOK {
		t.Fatalf("validate must not need a server: exit %d (%q)", code, stderr)
	}
	if strings.Contains(stderr, "not signed in") || strings.Contains(stderr, "Authentication") {
		t.Fatalf("validate reached the API: %q", stderr)
	}
}

// TestPluginDevNewScaffoldsFromTheEmbeddedTemplate: the template lives in the
// binary, not in the checkout, so this asserts against the real embedded files.
// The previous version of this test wrote its own skeleton fixture into a temp
// repo, which meant it kept passing while the command it guards was broken.
func TestPluginDevNewScaffoldsFromTheEmbeddedTemplate(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "plugins", "Plugins"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := run(t, nil, "", "plugin", "dev", "new", "comm-newthing", "--repo", repo)
	if code != ExitOK {
		t.Fatalf("new failed: exit %d, %q %q", code, out, stderr)
	}
	dest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-newthing")

	// Every file the template carries must land, with the .tmpl suffix stripped.
	for _, rel := range []string{
		"plugin.yaml", "go.mod", "README.md",
		"backend/main.go",
		"frontend/package.json", "frontend/tsconfig.json", "frontend/vite-plugin.config.ts",
		"frontend/src/index.tsx",
		"i18n/en/common.json",
		"icons/plugin.svg",
	} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("template file %s did not land: %v", rel, err)
		}
	}

	// No .tmpl name survives, and no file still names the template slug: a plugin
	// whose manifest, module path and bundle name disagree with its directory is
	// one the loader refuses.
	err := filepath.Walk(dest, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tmpl") {
			t.Errorf("a .tmpl file was written verbatim: %s", path)
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel := valueOr(filepath.Rel(dest, path))
		if strings.Contains(string(body), pluginTemplateSlug) {
			t.Errorf("%s still names the template slug %q", rel, pluginTemplateSlug)
		}
		if strings.Contains(string(body), strings.ReplaceAll(pluginTemplateSlug, "-", "_")) {
			t.Errorf("%s still names the underscore form of the template slug", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The four places the loader compares must all carry the new slug.
	assertContains(t, filepath.Join(dest, "plugin.yaml"), `slug:     "comm-newthing"`)
	assertContains(t, filepath.Join(dest, "go.mod"), "plugins/comm-newthing")
	assertContains(t, filepath.Join(dest, "backend", "main.go"), `Slug:    "comm-newthing"`)
	assertContains(t, filepath.Join(dest, "frontend", "vite-plugin.config.ts"), `const slug = 'comm-newthing'`)
	// The npm package name is slug-derived too, and was left behind by the old
	// directory-copy scaffold because it did not carry the leading underscore.
	assertContains(t, filepath.Join(dest, "frontend", "package.json"), "mcharbor-plugin-comm-newthing-frontend")

	// A second run must refuse rather than merge into the existing plugin.
	if code, _, stderr := run(t, nil, "", "plugin", "dev", "new", "comm-newthing", "--repo", repo); code != ExitUsage ||
		!strings.Contains(stderr, "already exists") {
		t.Fatalf("an existing plugin must be refused: exit %d, %q", code, stderr)
	}

	// A slug outside the packager's families is refused before anything is written.
	if code, _, stderr := run(t, nil, "", "plugin", "dev", "new", "mything", "--repo", repo); code != ExitUsage ||
		!strings.Contains(stderr, "must start with one of") {
		t.Fatalf("a bad slug prefix must be refused: exit %d, %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, "src", "plugins", "Plugins", "mything")); err == nil {
		t.Fatal("a refused slug still created a directory")
	}
}

// TestPluginTemplateIsComplete: the embed is a silent failure mode — a mistyped
// pattern yields an empty FS and scaffolding then writes nothing at all, which
// the command reports as success.
func TestPluginTemplateIsComplete(t *testing.T) {
	files, err := pluginTemplateFiles()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(files) < 10 {
		t.Fatalf("the embedded template carries %d file(s), expected at least 10: %v", len(files), names)
	}
	for _, want := range []string{"plugin.yaml", "go.mod", "backend/main.go", "frontend/src/index.tsx"} {
		if _, ok := files[want]; !ok {
			t.Errorf("the embedded template is missing %s; have %v", want, names)
		}
	}
}

// TestPluginTemplateGuardsItsRoot: a plugin's own React root is invisible to every
// host error boundary, so a scaffolded plugin must start with its root created
// through the onUncaughtError guard rather than a bare createRoot.
func TestPluginTemplateGuardsItsRoot(t *testing.T) {
	files, err := pluginTemplateFiles()
	if err != nil {
		t.Fatal(err)
	}
	index := string(files["frontend/src/index.tsx"])
	guard := string(files["frontend/src/mountGuard.tsx"])
	if guard == "" {
		t.Fatal("the template is missing frontend/src/mountGuard.tsx")
	}
	if strings.Contains(index, "createRoot(") || !strings.Contains(index, "guardedRoot(el)") {
		t.Error("frontend/src/index.tsx must create its root with guardedRoot(el), never a bare createRoot")
	}
	for _, want := range []string{"onUncaughtError", "t('pluginMount.failed', { ns: 'common' })"} {
		if !strings.Contains(guard, want) {
			t.Errorf("frontend/src/mountGuard.tsx is missing %q", want)
		}
	}
}

// assertContains fails when path does not hold want, naming the file.
func assertContains(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: %v", filepath.Base(path), err)
		return
	}
	if !strings.Contains(string(body), want) {
		t.Errorf("%s does not contain %q:\n%s", filepath.Base(path), want, string(body))
	}
}

// TestPluginDevBuildIsADryRunnableWrapper: the build verbs must drive the
// repository's own scripts rather than reimplement them, so --dry-run is the
// assertion that says which script and which arguments.
func TestPluginDevBuildIsADryRunnableWrapper(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "plugins", "Plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "development"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"catalog.sh", "build-plugin.sh"} {
		if err := os.WriteFile(filepath.Join(repo, "scripts", "development", script), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	code, out, stderr := run(t, nil, "", "plugin", "dev", "build", "--repo", repo, "--dry-run", "--all")
	if code != ExitOK {
		t.Fatalf("dry run failed: exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "scripts/development/catalog.sh") || !strings.Contains(out, "build --all") {
		t.Fatalf("build must drive catalog.sh: %q", out)
	}

	code, out, stderr = run(t, nil, "", "plugin", "dev", "package", "comm-fixture", "--repo", repo, "--dry-run")
	if code != ExitOK {
		t.Fatalf("dry run failed: exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "scripts/development/build-plugin.sh comm-fixture file:///mnt/plugin-catalog") {
		t.Fatalf("package must drive build-plugin.sh with the catalog URL: %q", out)
	}
}
