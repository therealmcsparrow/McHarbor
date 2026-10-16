// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// extract-core-plugin-symbols is a one-shot build-time tool used by
// docker/Dockerfile.plugin-packager. It walks every Go subdirectory
// of the core/plugin package (contract, services, types,
// specs/{storage,notification,identity,email,ai,snapshot}, host),
// enumerates every exported top-level declaration in each, and emits
// a single Go source file that force-links every type / var / const /
// func into the plugin's main package so the plugin .so's per-package
// ABI hashes match the host binary's.
//
// Why this exists: the Go plugin loader refuses a .so whose package
// ABI hash differs from the host's ("different version of package
// X"). The hash is computed from the set of symbols the linker keeps
// for each package — the plugin linker prunes symbols it cannot
// prove reachable from the plugin's exported entry points, so
// without force-linking the plugin's view of each shared package is
// a strict subset of the host's and the hashes diverge.
//
// Why we walk the subpackages: after the 2026-08-14 reach-set refactor
// (see .references/backup/2026-08-14-1900-shim-core-plugin/) the
// top-level core/plugin directory is a backward-compatibility shim
// whose only file is shim.go — every public symbol there is a
// `type X = contract.Y` alias. Type aliases are resolved at compile
// time; the linker tracks references to `core/plugin.X` but does NOT
// transitively pull in `core/plugin/contract.Y` from such a
// reference. So a force-link file generated against the shim
// directory alone cannot pin the per-package ABI hashes of the
// underlying contract / types / services / specs/* packages. This
// tool therefore walks each subpackage directly and emits
// per-package force-link blocks that reference the real symbols.
//
// The generated file is written to stdout; redirect it to a real
// path (NOT a path starting with `_` or `.` — the go tool ignores
// such files). Usage:
//
//	go run ./docker/cmd/extract-core-plugin-symbols \
//	    src/backend/core/plugin \
//	    > src/backend/_plugin_build/<slug>/backend/host_link_keep.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sym is one exported top-level declaration in one package.
type sym struct {
	name string
	kind string // "type", "var", "const", or "func"
}

// pkgSyms is the set of exported top-level symbols in one Go
// subpackage, together with the import path the plugin's generated
// file should use to reference it.
type pkgSyms struct {
	dir        string // filesystem path to the package directory
	importPath string // module-root-relative import path, e.g. "github.com/therealmcsparrow/mcharbor/core/plugin/types"
	syms       []sym
}

// keepPrefix is prepended to every force-link variable / alias so
// the generated symbols cannot collide with real source symbols
// (the plugin source is also compiled into the same package, so we
// need a name nobody would reasonably pick). The trailing `_`
// separator makes the generated identifier unambiguous.
const keepPrefix = "forceLinkMcHarborCorePlugin_"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: extract-core-plugin-symbols <pkg-root>")
		os.Exit(2)
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("abs %s: %w", os.Args[1], err))
		os.Exit(1)
	}
	importRoot, err := moduleImportRoot(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pkgs, err := collectAll(root, importRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(pkgs) == 0 {
		fmt.Fprintln(os.Stderr, "no Go subpackages found under", root)
		os.Exit(1)
	}
	// Stable order: subdirectory name. Same as `go list ./...` produces.
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].dir < pkgs[j].dir })
	emit(pkgs)
}

// moduleImportRoot derives the module-root-relative import path for
// files under root by walking up from root until a go.mod is found.
// The returned prefix is e.g. "github.com/therealmcsparrow/mcharbor".
// Returns an error if no go.mod is found within a reasonable walk.
func moduleImportRoot(root string) (string, error) {
	dir := root
	for range 16 {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			data, err := os.ReadFile(modPath)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", modPath, err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					name := strings.TrimSpace(strings.TrimPrefix(line, "module "))
					if name == "" {
						return "", fmt.Errorf("empty module path in %s", modPath)
					}
					rel, err := filepath.Rel(dir, root)
					if err != nil {
						return "", fmt.Errorf("rel %s vs %s: %w", root, dir, err)
					}
					rel = filepath.ToSlash(rel)
					if rel == "." {
						return name, nil
					}
					return name + "/" + rel, nil
				}
			}
			return "", fmt.Errorf("no `module` directive in %s", modPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", root)
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod walk depth exceeded from %s", root)
}

// collectAll walks root + every direct subdirectory of root that
// contains Go files and returns the symbols in each. The root
// directory itself is included if it contains Go files (e.g.
// core/plugin/shim.go). Subdirectories that contain no Go files
// (just nested subdirs like specs/) are recursed into — we want
// the leaf package directories, not the intermediate layout dirs.
func collectAll(root, importRoot string) ([]pkgSyms, error) {
	var out []pkgSyms
	// Include the root package itself if it has any Go files.
	rootSyms, err := collect(root)
	if err != nil {
		return nil, err
	}
	if len(rootSyms) > 0 {
		out = append(out, pkgSyms{
			dir:        root,
			importPath: importRoot,
			syms:       rootSyms,
		})
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip non-Go directories (data dirs, manifest JSON, etc.).
		// We recurse into package directories so a layout dir like
		// specs/ that only contains subpackage dirs is handled
		// correctly.
		sub := filepath.Join(root, entry.Name())
		subPkgs, err := collectAll(sub, importRoot+"/"+entry.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, subPkgs...)
	}
	return out, nil
}

// collect parses every .go file in dir and returns the exported
// top-level declarations. Grouped declarations (type (...), var (...),
// const (...)) are flattened. Method declarations (func (r *Recv)
// M(...)) are intentionally skipped — their existence is implied by
// referencing the receiver type.
//
// We walk files individually instead of using parser.ParseDir
// because the latter does not consider build tags and is deprecated
// in Go 1.26. We do a minimal build-tag filter ourselves: skip
// files whose first non-comment line is `//go:build` or `// +build`
// and contains a tag the build doesn't enable.
func collect(dir string) ([]sym, error) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	seen := map[string]bool{}
	var out []sym
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		if !buildTagEnabled(path) {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				// Skip methods — they're linked when the receiver
				// type is referenced.
				if d.Recv != nil {
					continue
				}
				if d.Name.IsExported() {
					add(&out, seen, d.Name.Name, "func")
				}
			case *ast.GenDecl:
				var kind string
				switch d.Tok {
				case token.TYPE:
					kind = "type"
				case token.VAR:
					kind = "var"
				case token.CONST:
					kind = "const"
				default:
					continue // import, etc.
				}
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							add(&out, seen, s.Name.Name, kind)
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								add(&out, seen, name.Name, kind)
							}
						}
					}
				}
			}
		}
	}
	return out, nil
}

// buildTagEnabled reports whether the file should be compiled in the
// current build context. We accept everything that doesn't have a
// build tag, and we accept files with an unconditional `//go:build`
// (no tag expression or only `ignore` flags). This is intentionally
// permissive — the packager runs in a controlled environment and
// core/plugin does not use build tags, so the bar is just "don't
// skip files we need".
func buildTagEnabled(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	// Look for `//go:build` line in the leading comment block.
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:build ") {
			return true
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Hit a non-comment line — no go:build directive.
		return true
	}
	return true
}

func add(out *[]sym, seen map[string]bool, name, kind string) {
	if seen[name] {
		return
	}
	seen[name] = true
	*out = append(*out, sym{name: name, kind: kind})
}

// shortImport derives a Go-package identifier from an import path.
// The last `/`-separated segment wins; if it ends in `_test` or
// contains a `.` (a versioned path), we keep only the alphanumeric
// part. The result is safe to use as a variable name prefix.
func shortImport(importPath string) string {
	seg := importPath
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	seg = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, seg)
	if seg == "" {
		seg = "pkg"
	}
	return seg
}

// emit writes the force-link Go source to stdout. The file is
// structured so the package-level declarations are the force-link
// mechanism — they cannot be pruned because they are not inside a
// function. One block is emitted per subpackage so each block
// contributes to the per-package reach set of the import it
// references; the slices are then cross-referenced from a final
// package-level var to ensure the linker has an observable read of
// every entry.
func emit(pkgs []pkgSyms) {
	fmt.Println("// Code generated by docker/cmd/extract-core-plugin-symbols. DO NOT EDIT.")
	fmt.Println("//")
	fmt.Println("// Force-link every exported top-level symbol of every Go subpackage")
	fmt.Println("// under github.com/therealmcsparrow/mcharbor/core/plugin/* so each")
	fmt.Println("// subpackage's per-package ABI hash in the plugin .so matches the")
	fmt.Println("// host binary's. Without this, the plugin linker prunes symbols the")
	fmt.Println("// plugin does not directly use from each shared package, producing a")
	fmt.Println("// strict-subset view that fails to load with \"different version of")
	fmt.Println("// package <subpkg>\".")
	fmt.Println()
	fmt.Println("package main")
	fmt.Println()
	fmt.Println("import (")
	fmt.Println("\t\"reflect\"")
	// Imports — one per subpackage with symbols.
	used := map[string]bool{}
	aliases := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if len(p.syms) == 0 {
			continue
		}
		alias := shortImport(p.importPath)
		// Disambiguate collisions (e.g. two packages named "host" vs
		// a different layout with the same short name).
		base := alias
		i := 2
		for used[alias] {
			alias = fmt.Sprintf("%s%d", base, i)
			i++
		}
		used[alias] = true
		aliases = append(aliases, alias)
		fmt.Printf("\t%s %q\n", alias, p.importPath)
	}
	fmt.Println(")")
	fmt.Println()

	// Per-package: types → slice of reflect.Type; non-types → real-name
	// var assignment. Emit both kinds so each package's reach set is
	// padded to match the host's.
	allSlices := make([]string, 0, len(pkgs))
	for i, p := range pkgs {
		if len(p.syms) == 0 {
			continue
		}
		alias := aliases[i]
		sliceName := fmt.Sprintf("%s_forceLinkTypes", alias)

		var types, vals []sym
		for _, s := range p.syms {
			switch s.kind {
			case "type":
				types = append(types, s)
			default:
				vals = append(vals, s)
			}
		}

		if len(types) > 0 {
			fmt.Printf("// --- %s (types) ---\n", p.importPath)
			fmt.Printf("// reflect.TypeOf((*X)(nil)).Elem() is a runtime call the linker")
			fmt.Printf("\n// cannot constant-fold, so the type descriptor for X is reachable")
			fmt.Printf("\n// from the package-level var's initializer and survives into the")
			fmt.Printf("\n// .so. The slice is assigned to a package-level var so the linker")
			fmt.Printf("\n// has an observable read of every entry.\n")
			fmt.Printf("var %s = []reflect.Type{\n", sliceName)
			for _, s := range types {
				fmt.Printf("\treflect.TypeOf((*%s.%s)(nil)).Elem(),\n", alias, s.name)
			}
			fmt.Println("}")
			fmt.Println()
			allSlices = append(allSlices, sliceName)
		}
		if len(vals) > 0 {
			fmt.Printf("// --- %s (funcs / vars / consts) ---\n", p.importPath)
			fmt.Printf("// Assigning the value to a package-level variable with a real")
			fmt.Printf("\n// name (not `_`) forces the linker to keep the symbol's entry in")
			fmt.Printf("\n// the compiled package.\n")
			for _, s := range vals {
				fmt.Printf("var %s%s_%s = %s.%s\n", keepPrefix, alias, s.name, alias, s.name)
			}
			fmt.Println()
		}
	}

	// Final cross-reference: a package-level var whose initializer
	// references every per-package types slice. This gives the linker
	// one observable read of every entry so the slices themselves
	// cannot be pruned even if the host's loader never reaches them.
	if len(allSlices) > 0 {
		fmt.Println("// --- cross-reference ---")
		fmt.Println("// forceLinkAll references every per-package types slice so the")
		fmt.Println("// linker has an observable read of each entry. The slice is never")
		fmt.Println("// read at runtime; only the linker sees it.")
		fmt.Println("var forceLinkAll = [][]reflect.Type{")
		for _, s := range allSlices {
			fmt.Printf("\t%s,\n", s)
		}
		fmt.Println("}")
		fmt.Println()
	}

	fmt.Println("// init is a deliberate no-op. The force-link work happens at")
	fmt.Println("// package-init time via the var initializers above.")
	fmt.Println("func init() {}")
}
