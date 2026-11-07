// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versionorder

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type siteTable struct {
	Sites []struct {
		File     string `yaml:"file"`
		Name     string `yaml:"name"`
		Kind     string `yaml:"kind"`
		TestedBy string `yaml:"tested_by"`
	} `yaml:"sites"`
}

// comparatorDecl matches a declaration of a function named like a version comparator
// in Go, TypeScript or Python; a call or a mention in prose never matches.
var comparatorDecl = regexp.MustCompile(
	`(?m)^\s*(?:func\s+(?:\([^)]*\)\s*)?|(?:export\s+)?(?:async\s+)?function\s+|(?:export\s+)?(?:const|let)\s+|def\s+)` +
		`(compareVersion\w*|compareAgent\w*|\w+AtLeast|isNewer\w*|newer|semverTuple|releaseParts|compare_versions)\b`)

// pythonCompare is the one comparator in catalog_entry.py whose name does not follow the pattern.
var pythonCompare = regexp.MustCompile(`(?m)^def (compare)\(`)

var skipDirs = map[string]bool{"node_modules": true, "dist": true, "build": true, "vendor": true, ".git": true}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}

func loadSites(t *testing.T, root string) siteTable {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "src", "contracts", "versionorder", "sites.yaml"))
	if err != nil {
		t.Fatalf("read sites: %v", err)
	}
	var tbl siteTable
	if err := yaml.Unmarshal(raw, &tbl); err != nil {
		t.Fatalf("parse sites: %v", err)
	}
	if len(tbl.Sites) == 0 {
		t.Fatal("sites table is empty")
	}
	return tbl
}

func isSource(rel string) bool {
	if strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".test.ts") || strings.HasSuffix(rel, ".test.tsx") {
		return false
	}
	switch filepath.Ext(rel) {
	case ".go", ".ts", ".tsx":
		return true
	}
	return false
}

// foundComparators returns "file#name" for every comparator declared under src and
// in catalog_entry.py; it only reads files and never runs what it finds.
func foundComparators(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	scan := func(rel string, patterns ...*regexp.Regexp) {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			return
		}
		seen := map[string]bool{}
		for _, p := range patterns {
			for _, m := range p.FindAllStringSubmatch(string(raw), -1) {
				if k := rel + "#" + m[1]; !seen[k] {
					seen[k] = true
					found = append(found, k)
				}
			}
		}
	}
	err := filepath.WalkDir(filepath.Join(root, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if isSource(rel) {
			scan(rel, comparatorDecl)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk src: %v", err)
	}
	scan("scripts/development/catalog_entry.py", comparatorDecl, pythonCompare)
	sort.Strings(found)
	return found
}

// TestEverySiteIsListed fails on a version comparator that sites.yaml does not name,
// so a new copy of the order cannot appear without a place in the contract.
func TestEverySiteIsListed(t *testing.T) {
	root := repoRoot(t)
	tbl := loadSites(t, root)
	listed := map[string]bool{}
	for _, s := range tbl.Sites {
		listed[s.File+"#"+s.Name] = true
	}
	found := foundComparators(t, root)
	if len(found) == 0 {
		t.Fatal("scan found no comparator at all; the pattern or the root is wrong")
	}
	for _, k := range found {
		if !listed[k] {
			t.Errorf("comparator %s is not in src/contracts/versionorder/sites.yaml", k)
		}
	}
}

// TestListedSitesExist fails on a sites.yaml row whose function or test file is gone,
// so the table cannot keep naming a comparator that was deleted or moved.
func TestListedSitesExist(t *testing.T) {
	root := repoRoot(t)
	for _, s := range loadSites(t, root).Sites {
		switch s.Kind {
		case "reference", "copy", "delegates":
		default:
			t.Errorf("%s#%s: kind %q is not reference, copy or delegates", s.File, s.Name, s.Kind)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(s.File)))
		if err != nil {
			t.Errorf("%s#%s: %v", s.File, s.Name, err)
			continue
		}
		if !strings.Contains(string(raw), s.Name+"(") && !strings.Contains(string(raw), s.Name+" =") && !strings.Contains(string(raw), s.Name+" :=") {
			t.Errorf("%s does not declare %s", s.File, s.Name)
		}
		if s.TestedBy == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(s.TestedBy))); err != nil {
			t.Errorf("%s#%s: tested_by %s: %v", s.File, s.Name, s.TestedBy, err)
		}
	}
}

// TestComparatorPatternSeesTheAgentAndAtLeastFamily proves the scan reads a new agent or
// at-least comparator, so one added without a row in sites.yaml fails TestEverySiteIsListed.
func TestComparatorPatternSeesTheAgentAndAtLeastFamily(t *testing.T) {
	for _, decl := range []string{
		"func compareAgentThing(a, b string) int {",
		"func compareAgentProtocolVersion(a, b []string) int {",
		"func (p *Pool) agentAtLeast(a, b string) bool {",
		"func releaseAtLeast(current, floor string) bool {",
		"func newer(v1, b1, v2, b2 string) bool {",
		"export function isNewerBuild(a: string) {",
	} {
		if !comparatorDecl.MatchString(decl) {
			t.Errorf("comparatorDecl does not match %q", decl)
		}
	}
	for _, call := range []string{"	if compareAgentThing(a, b) > 0 {", "	ok := agentAtLeast(a, b)", "// newer(a, b) reads the order"} {
		if comparatorDecl.MatchString(call) {
			t.Errorf("comparatorDecl matches a call or comment: %q", call)
		}
	}
}
