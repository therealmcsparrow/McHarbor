// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// pluginTemplateFS carries the new-plugin scaffold inside the binary, so
// `plugin dev new` does not depend on a template directory surviving in the
// checkout — which is how it broke the last time one was renamed.
//
// Every file is stored with a .tmpl suffix. Without it the template's own
// go.mod would make this directory a second module and its main.go a package
// the toolchain tries to compile, and go:embed refuses to cross a module
// boundary at all.
//
//go:embed templates/plugin
var pluginTemplateFS embed.FS

// pluginTemplateRoot is the embed root; fs paths are always slash-separated.
const pluginTemplateRoot = "templates/plugin"

// pluginTemplateSlug is the placeholder every template file names, rewritten to
// the author's slug at scaffold time. It carries no leading underscore because
// the embed directive silently skips `_`-prefixed paths.
const pluginTemplateSlug = "example-plugin"

// pluginTemplateFiles returns the template keyed by its scaffolded relative
// path, with the .tmpl suffix already stripped. It exists so a mistyped embed
// pattern is a test failure rather than a scaffold that writes nothing and
// reports success.
func pluginTemplateFiles() (map[string][]byte, error) {
	out := make(map[string][]byte)
	err := fs.WalkDir(pluginTemplateFS, pluginTemplateRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := relSlash(pluginTemplateRoot, p)
		if err != nil {
			return err
		}
		body, err := pluginTemplateFS.ReadFile(p)
		if err != nil {
			return err
		}
		out[strings.TrimSuffix(rel, ".tmpl")] = body
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// renderPluginTemplate rewrites the placeholder slug to slug, in both the
// hyphenated form the manifest and module path use and the underscored form the
// UMD global and JS identifiers use.
func renderPluginTemplate(body []byte, slug string) []byte {
	text := strings.ReplaceAll(string(body), pluginTemplateSlug, slug)
	text = strings.ReplaceAll(text,
		strings.ReplaceAll(pluginTemplateSlug, "-", "_"),
		strings.ReplaceAll(slug, "-", "_"))
	return []byte(text)
}

// relSlash is path.Rel for slash paths, which the standard library omits.
func relSlash(base, target string) (string, error) {
	base = path.Clean(base)
	target = path.Clean(target)
	if base == target {
		return ".", nil
	}
	if !strings.HasPrefix(target, base+"/") {
		return "", errNotUnderRoot{base: base, target: target}
	}
	return strings.TrimPrefix(target, base+"/"), nil
}

// errNotUnderRoot reports an embed walk that left the template root, which
// would mean the embed pattern changed without this file changing with it.
type errNotUnderRoot struct{ base, target string }

func (e errNotUnderRoot) Error() string {
	return e.target + " is not under the template root " + e.base
}
