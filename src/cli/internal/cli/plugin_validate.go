// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// pluginFinding is one verdict about one plugin source tree. Severity is
// "error" when the plugin would misbehave once installed, "warn" when it
// would work but not as its author intended.
type pluginFinding struct {
	Slug     string `json:"slug"`
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Detail   string `json:"detail"`
}

// pluginManifest is the subset of plugin.yaml this validator reads. It is a
// local copy rather than an import because the CLI is its own Go module and
// must not take a dependency on the host's plugin contract package.
type pluginManifest struct {
	Schema        string                 `yaml:"schema"`
	Slug          string                 `yaml:"slug"`
	Name          string                 `yaml:"name"`
	Version       string                 `yaml:"version"`
	Mcharbor      string                 `yaml:"mcharbor"`
	AddsPerms     []pluginPermissionDecl `yaml:"adds_permissions"`
	ConsumesPerms []string               `yaml:"consumes_permissions"`
	Caps          pluginCapabilities     `yaml:"capabilities"`
	Contribs      pluginContributes      `yaml:"contributes"`
}

type pluginPermissionDecl struct {
	Key   string `yaml:"key"`
	Scope string `yaml:"scope"`
}

type pluginCapabilities struct {
	NetBind bool `yaml:"net_bind"`
	FSWrite bool `yaml:"fs_write"`
}

type pluginContributes struct {
	Routes       []pluginRouteDecl `yaml:"routes"`
	Nodes        []string          `yaml:"nodes"`
	Capabilities []pluginCapDecl   `yaml:"capabilities"`
	Forms        []pluginFormDecl  `yaml:"forms"`
}

// pluginFormDecl is one contributes.forms slot; a channel slot's key is the channel type
// the plugin registers, the only one its capabilities may speak for.
type pluginFormDecl struct {
	Resource string `yaml:"resource"`
	Kind     string `yaml:"kind"`
	Key      string `yaml:"key"`
}

type pluginRouteDecl struct {
	Method        string `yaml:"method"`
	Path          string `yaml:"path"`
	Requires      string `yaml:"requires_permission"`
	Public        bool   `yaml:"public"`
	AuditSeverity string `yaml:"audit_severity"`
}

// pluginAuditSeverities mirrors the host's KnownAuditSeverity (core/plugin/host), which the
// loader enforces; both are checked against src/contracts/pluginroutes/audit_severity.yaml.
var pluginAuditSeverities = []string{"info", "success", "warning", "error"}

// The three registration shapes the in-tree plugins use, copied from the host's
// route_declaration_drift_test.go so the CLI and the host agree about what a
// registered route looks like. A plugin inventing a fourth shape is reported by
// the zero-registrations check rather than passing silently.
var (
	rePluginRegCall      = regexp.MustCompile(`\breg\.(Protected|Public|Auth)\(\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"`)
	rePluginLiteralBool  = regexp.MustCompile(`\{\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"\s*,\s*(true|false)\s*,`)
	rePluginLiteralPlain = regexp.MustCompile(`\{\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"\s*,\s*\(\*`)
	// A Version: "…" field in the plugin's Go entry point, which the loader
	// asserts against the manifest.
	rePluginVersionSymbol = regexp.MustCompile(`(?m)^\s*Version:\s*"([^"]+)"`)
	// A require line in a go.mod, single or block form.
	reGoModRequire = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([a-zA-Z0-9.\-_/]+\.[a-zA-Z0-9.\-_/]+)\s+(v[^\s/]+)`)
	// Permission keys are dot-separated lowercase words; a hyphen or a colon is
	// refused at install time, which is what blocked every comm plugin.
	rePermissionKey = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)+$`)
)

// pluginRegisteredRoute is one route a plugin's Go source hands to the registry.
type pluginRegisteredRoute struct {
	method string
	path   string
	public bool
	file   string
}

// validatePlugin checks one plugin source tree and returns every finding, worst
// first. repoRoot is the repository root; slug names a directory under
// src/plugins/Plugins.
func validatePlugin(repoRoot, slug string) ([]pluginFinding, error) {
	dir := filepath.Join(repoRoot, "src", "plugins", "Plugins", slug)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, usageError("no plugin source at %s", dir)
	}

	var out []pluginFinding
	add := func(severity, check, detail string) {
		out = append(out, pluginFinding{Slug: slug, Severity: severity, Check: check, Detail: detail})
	}

	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, usageError("%s/plugin.yaml is not readable: %v", slug, err)
	}
	var m pluginManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		add("error", "manifest", "plugin.yaml does not parse: "+err.Error())
		return out, nil //nolint:nilerr // an unparseable manifest is reported as a finding, not as a command failure
	}

	validateManifestShape(slug, m, add)
	validateCapabilities(slug, m, add)
	validateGoMod(dir, repoRoot, add)
	validateVersionSymbol(dir, m, add)
	validateIcons(dir, add)
	validateFrontendFreshness(dir, slug, add)
	validateRouteDrift(dir, m, add)

	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity < out[j].Severity })
	return out, nil
}

type findingSink func(severity, check, detail string)

// reservedPluginSlugs mirrors core/pluginslug in the backend, which the installer and
// loader enforce; both are checked against src/contracts/pluginslugs/reserved.yaml.
var reservedPluginSlugs = []string{"host"}

func validateManifestShape(slug string, m pluginManifest, add findingSink) {
	if slices.Contains(reservedPluginSlugs, strings.ToLower(strings.TrimSpace(m.Slug))) {
		add("error", "manifest", "slug "+quoteOrEmpty(m.Slug)+" is reserved by the host; the installer and the loader refuse it")
	}
	if m.Slug != slug {
		add("error", "manifest", "plugin.yaml declares slug "+quoteOrEmpty(m.Slug)+
			" but the directory is "+slug+"; the loader compares them")
	}
	if m.Schema != "mcharbor.plugin/v1" {
		add("error", "manifest", "schema must be mcharbor.plugin/v1, got "+quoteOrEmpty(m.Schema))
	}
	switch {
	case m.Version == "":
		add("error", "manifest", "no version: line; the tarball name, the catalog entry and the payload all read it")
	case strings.ContainsAny(m.Version, `/\ "'`):
		add("error", "manifest", "version "+quoteOrEmpty(m.Version)+" contains characters that are not safe in a filename")
	}
	if m.Mcharbor == "" {
		add("warn", "manifest", "no mcharbor: range; nothing states which host releases this plugin needs")
	}
	if m.Caps.NetBind {
		add("error", "capabilities", "net_bind is hard-vetoed by the loader; the plugin will never boot")
	}
	if m.Caps.FSWrite {
		add("error", "capabilities", "fs_write is hard-vetoed outside the plugin data dir; the plugin will never boot")
	}
	for _, rt := range m.Contribs.Routes {
		if rt.AuditSeverity != "" && !slices.Contains(pluginAuditSeverities, rt.AuditSeverity) {
			add("error", "routes", rt.Method+" "+rt.Path+" declares audit_severity "+quoteOrEmpty(rt.AuditSeverity)+
				"; the host refuses the manifest unless it is info, success, warning or error")
		}
	}
	for _, p := range m.AddsPerms {
		if !rePermissionKey.MatchString(p.Key) {
			add("error", "permissions", "adds_permissions key "+quoteOrEmpty(p.Key)+
				" is not <category>.<action> in lowercase with underscores; install refuses a hyphen or a colon")
		}
		// A plugin's own scope vocabulary is narrower than the host catalog's:
		// manifest_schema.go accepts global, env and stack, and nothing else. It
		// is "env" here and "environment" in the host's own CatalogSeed, which is
		// exactly the confusion worth failing on.
		switch p.Scope {
		case "global", "env", "stack":
		case "":
			add("warn", "permissions", "adds_permissions key "+quoteOrEmpty(p.Key)+
				" has no scope; the manifest allows it, but nothing then says how narrowly the key applies")
		default:
			add("error", "permissions", "adds_permissions key "+quoteOrEmpty(p.Key)+
				" has scope "+quoteOrEmpty(p.Scope)+
				"; a plugin manifest accepts only global, env or stack — \"environment\" is the host catalog's word, not the manifest's")
		}
	}
}

// validateGoMod refuses a CRLF go.mod and a dependency version that disagrees
// with the host's. Both silently break the staged .so: the packager reads go.mod
// as text, and plugin.Open rejects a package built against a different version.
func validateGoMod(dir, repoRoot string, add findingSink) {
	raw, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		add("error", "go.mod", "not readable: "+err.Error())
		return
	}
	if strings.Contains(string(raw), "\r\n") {
		add("error", "go.mod", "has CRLF line endings; the packager reads it as text and stages a broken .so. "+
			"Rewrite it with LF endings — this repo is an LF tree")
		return
	}
	hostRaw, err := os.ReadFile(filepath.Join(repoRoot, "src", "backend", "go.mod"))
	if err != nil {
		return
	}
	host := goModRequires(string(hostRaw))
	for module, version := range goModRequires(string(raw)) {
		want, shared := host[module]
		if shared && want != version {
			add("error", "go.mod", module+" is "+version+" here and "+want+
				" in src/backend/go.mod; a shared dependency at two versions stages a .so the host refuses to open")
		}
	}
}

// goModRequires maps each required module to its version. The replace directive
// is deliberately not followed: only the recorded versions matter to the ABI.
func goModRequires(src string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "replace") ||
			strings.HasPrefix(trimmed, "module") || strings.HasPrefix(trimmed, "exclude") {
			continue
		}
		if match := reGoModRequire.FindStringSubmatch(line); match != nil {
			out[match[1]] = match[2]
		}
	}
	return out
}

// validateVersionSymbol checks the Go entry point's Version field against the
// manifest. The loader asserts the manifest, the directory name and this symbol
// all agree, and a source edit moves only two of the three.
func validateVersionSymbol(dir string, m pluginManifest, add findingSink) {
	raw, err := os.ReadFile(filepath.Join(dir, "backend", "main.go"))
	if err != nil {
		return
	}
	match := rePluginVersionSymbol.FindStringSubmatch(string(raw))
	if match == nil {
		add("warn", "version", "backend/main.go has no Version: field the loader can assert against the manifest")
		return
	}
	if m.Version != "" && match[1] != m.Version {
		add("error", "version", "backend/main.go says "+match[1]+" and plugin.yaml says "+m.Version+
			"; the loader refuses a plugin whose symbol and manifest disagree")
	}
}

func validateIcons(dir string, add findingSink) {
	if _, err := os.Stat(filepath.Join(dir, "icons", "plugin.svg")); err != nil {
		add("error", "icons", "no icons/plugin.svg; the packager treats a missing brand icon as fatal")
	}
}

// validateFrontendFreshness compares the committed UMD bundle against the
// sources that produce it. An edit under frontend/src ships nothing at all until
// npm run build has run in that plugin, and the symptom appears in the browser
// as "the plugin did not export a mount() function".
func validateFrontendFreshness(dir, slug string, add findingSink) {
	bundle, err := os.Stat(filepath.Join(dir, "frontend", slug+".umd.cjs"))
	if err != nil {
		if _, srcErr := os.Stat(filepath.Join(dir, "frontend", "src")); srcErr == nil {
			add("error", "frontend", "frontend/src exists but frontend/"+slug+
				".umd.cjs does not; run npm run build in frontend/ — the packager copies the bundle, it does not build it")
		}
		return
	}
	newest := ""
	var newestTime = bundle.ModTime()
	err = filepath.Walk(filepath.Join(dir, "frontend", "src"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // an unreadable source file is skipped; the staleness check only needs the newest readable one
		}
		if info.ModTime().After(newestTime) {
			newest, newestTime = path, info.ModTime()
		}
		return nil
	})
	if err == nil && newest != "" {
		add("warn", "frontend", filepath.Base(newest)+" is newer than frontend/"+slug+
			".umd.cjs; run npm run build in frontend/ or the edit ships nothing (mtimes survive an edit, not a checkout)")
	}
}

// validateRouteDrift is the check this command exists for. A plugin that
// registers a non-public route its plugin.yaml never declares gets no permission
// middleware at all: lookupRouteDecl returns an empty permission on a miss and
// the middleware short-circuits an empty key to a pass-through, so the route is
// reachable by any authenticated user whatever their role.
func validateRouteDrift(dir string, m pluginManifest, add findingSink) {
	routes, err := scanPluginRegistrations(filepath.Join(dir, "backend"))
	if err != nil {
		return
	}
	for _, rt := range routes {
		if rt.public {
			continue
		}
		requires, declaredPublic, found := findPluginRouteDecl(m, rt.method, rt.path)
		switch {
		case !found:
			add("error", "routes", rt.method+" "+rt.path+" is registered non-public in backend/"+rt.file+
				" and plugin.yaml does not declare it; it has no permission middleware and any"+
				" authenticated user can reach it")
		case declaredPublic:
			// The manifest overrides the registration to public. Legal, and the
			// loader honours it; nothing to enforce.
		case strings.TrimSpace(requires) == "":
			add("error", "routes", rt.method+" "+rt.path+
				" is declared with an empty requires_permission; the middleware short-circuits an"+
				" empty key to a pass-through")
		}
	}
	for _, decl := range m.Contribs.Routes {
		if !registrationExists(routes, decl.Method, decl.Path) {
			add("warn", "routes", strings.ToUpper(decl.Method)+" "+decl.Path+
				" is declared in plugin.yaml but no registration in backend/ matches it; the declaration is dead")
		}
	}
}

// findPluginRouteDecl mirrors routeRegistry.lookupRouteDecl: case-insensitive on
// the method, exact on the path, because that is what the loader compares at
// mount time.
func findPluginRouteDecl(m pluginManifest, method, path string) (requires string, public bool, found bool) {
	for _, rt := range m.Contribs.Routes {
		if strings.EqualFold(rt.Method, method) && rt.Path == path {
			return rt.Requires, rt.Public, true
		}
	}
	return "", false, false
}

func registrationExists(routes []pluginRegisteredRoute, method, path string) bool {
	for _, rt := range routes {
		if strings.EqualFold(rt.method, method) && rt.path == path {
			return true
		}
	}
	return false
}

// scanPluginRegistrations reads every non-test .go file in a plugin's backend
// directory and returns the routes it hands to the registry.
func scanPluginRegistrations(dir string) ([]pluginRegisteredRoute, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []pluginRegisteredRoute
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		src := string(body)
		for _, match := range rePluginRegCall.FindAllStringSubmatch(src, -1) {
			out = append(out, pluginRegisteredRoute{
				method: strings.ToUpper(match[2]), path: match[3],
				public: match[1] == "Public", file: f.Name(),
			})
		}
		for _, match := range rePluginLiteralBool.FindAllStringSubmatch(src, -1) {
			out = append(out, pluginRegisteredRoute{
				method: strings.ToUpper(match[1]), path: match[2],
				public: match[3] == "true", file: f.Name(),
			})
		}
		for _, match := range rePluginLiteralPlain.FindAllStringSubmatch(src, -1) {
			out = append(out, pluginRegisteredRoute{
				method: strings.ToUpper(match[1]), path: match[2], file: f.Name(),
			})
		}
	}
	return out, nil
}

func quoteOrEmpty(s string) string {
	if s == "" {
		return "an empty value"
	}
	return `"` + s + `"`
}
