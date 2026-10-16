// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Plugin commands are declared by the server and executed by this binary, so
// the tree a given operator sees depends on which plugins their instance has
// installed. The spec is cached on disk and the tree is built from the cache
// alone: building it must cost nothing and must work with no network, because it
// happens on every single invocation, `--help` included.

// pluginSpecVersion is the contract version this binary understands. A server
// serving a higher number is ignored entirely and reported by `doctor`, rather
// than half-read into a tree whose commands might mean something else.
const pluginSpecVersion = 1

// pluginSpec is the server's answer, plus what the cache needs to revalidate it.
type pluginSpec struct {
	SpecVersion int                `json:"specVersion"`
	Build       string             `json:"build"`
	Roots       []pluginRoot       `json:"roots"`
	Commands    []pluginCLICommand `json:"commands"`
	ETag        string             `json:"-"`
	FetchedAt   time.Time          `json:"-"`
}

// pluginRoot is one plugin's top-level command group.
type pluginRoot struct {
	Slug    string   `json:"plugin_slug"`
	Name    string   `json:"name"`
	Short   string   `json:"short"`
	Aliases []string `json:"aliases"`
}

// pluginCLICommand is one declared command.
type pluginCLICommand struct {
	Slug      string         `json:"plugin_slug"`
	Path      []string       `json:"path"`
	Kind      string         `json:"kind"`
	Method    string         `json:"method"`
	Route     string         `json:"route"`
	Args      []string       `json:"args"`
	Flags     []pluginFlag   `json:"flags"`
	Columns   []pluginColumn `json:"columns"`
	Body      string         `json:"body"`
	Needs     string         `json:"needs"`
	Risk      string         `json:"risk"`
	GuestKind string         `json:"guest_kind"`
	Consoles  []string       `json:"consoles"`
	Short     string         `json:"short"`
	Long      string         `json:"long"`
	Example   string         `json:"example"`
	EnvScoped bool           `json:"env_scoped"`
}

// pluginFlag is one flag on a declared command.
type pluginFlag struct {
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Target   string `json:"Target"`
	Field    string `json:"Field"`
	Required bool   `json:"Required"`
	Default  string `json:"Default"`
	Help     string `json:"Help"`
}

// pluginColumn is one column of a declared rendering.
type pluginColumn struct {
	Header string `json:"Header"`
	Path   string `json:"Path"`
	Format string `json:"Format"`
}

// cachedSpec is what is written to disk: the spec plus its validator.
type cachedSpec struct {
	Spec      pluginSpec `json:"spec"`
	ETag      string     `json:"etag"`
	FetchedAt time.Time  `json:"fetchedAt"`
}

// The kinds this binary can execute. A command naming anything else is skipped
// with a count in `doctor` rather than failing the tree, so a plugin built
// against a newer host does not make an older CLI unusable.
var knownPluginKinds = map[string]bool{
	"list": true, "get": true, "action": true, "text": true,
	"upload": true, "download": true, "attach": true,
}

// pluginSpecDisabled reports whether the operator has switched the surface off.
// A CI job that wants the tree frozen to what this binary itself ships sets it.
func pluginSpecDisabled() bool {
	return os.Getenv("MCHARBOR_NO_PLUGIN_CLI") == "1"
}

// specPath is where one context's cached spec lives. The context name is
// sanitised because it is operator-chosen and ends up in a filename.
func specPath(contextName string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	name := sanitiseContextName(contextName)
	if name == "" {
		name = "default"
	}
	return filepath.Join(dir, "cli-spec", name+".json"), nil
}

var unsafeContextChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitiseContextName reduces a context name to something safe in a path. A
// context called `../../etc/passwd` is a context name, not a path.
func sanitiseContextName(name string) string {
	return strings.Trim(unsafeContextChars.ReplaceAllString(name, "-"), "-.")
}

// loadPluginSpec reads one context's cached spec. A missing or unreadable cache
// is not an error: the tree is simply core-only until a call populates it.
func loadPluginSpec(contextName string) *pluginSpec {
	if pluginSpecDisabled() {
		return nil
	}
	path, err := specPath(contextName)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// A corrupt or unreadable cache must not break every command, so it
			// is treated exactly as an absent one.
			return nil
		}
		return nil
	}
	var cached cachedSpec
	if err := json.Unmarshal(raw, &cached); err != nil {
		return nil
	}
	if cached.Spec.SpecVersion != pluginSpecVersion {
		return nil
	}
	spec := cached.Spec
	spec.ETag = cached.ETag
	spec.FetchedAt = cached.FetchedAt
	return &spec
}

// savePluginSpec writes the cache with owner-only permissions: it names a
// server's plugins, which is nobody else's business.
func savePluginSpec(contextName string, spec pluginSpec) error {
	path, err := specPath(contextName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(cachedSpec{Spec: spec, ETag: spec.ETag, FetchedAt: spec.FetchedAt}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the plugin command spec: %w", err)
	}
	return os.WriteFile(path, raw, 0o600)
}

// fetchPluginSpec asks the server for the command spec. etag is sent as
// If-None-Match; a 304 returns (nil, nil), which is the steady state.
func fetchPluginSpec(ctx context.Context, a *API, etag string) (*pluginSpec, error) {
	headers := map[string]string{}
	if etag != "" {
		headers["If-None-Match"] = etag
	}
	resp, err := a.rawGet(ctx, "/api/plugins/loader/cli", nil, headers)
	if err != nil {
		return nil, transportError("pluginLoaderCLI", err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	data, err := a.do("pluginLoaderCLI", resp, nil)
	if err != nil {
		return nil, err
	}
	var spec pluginSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("decoding the plugin command spec: %w", err)
	}
	spec.ETag = resp.Header.Get("ETag")
	spec.FetchedAt = time.Now()
	return &spec, nil
}

// refreshPluginSpec fetches and caches the spec, and reports what changed so an
// operator learns a new root appeared without having to ask.
func refreshPluginSpec(ctx context.Context, g *globals) (*pluginSpec, string, error) {
	a, err := g.api()
	if err != nil {
		return nil, "", err
	}
	before := loadPluginSpec(a.target.ContextName)
	etag := ""
	if before != nil {
		etag = before.ETag
	}
	spec, err := fetchPluginSpec(ctx, a, etag)
	if err != nil {
		return nil, "", err
	}
	if spec == nil {
		return before, "", nil
	}
	if spec.SpecVersion != pluginSpecVersion {
		return nil, "", &Error{Code: ExitRemote, Operation: "pluginLoaderCLI",
			Message: fmt.Sprintf("the server serves plugin command spec version %d and this CLI understands %d; upgrade the CLI",
				spec.SpecVersion, pluginSpecVersion)}
	}
	if err := savePluginSpec(a.target.ContextName, *spec); err != nil {
		return spec, "", err
	}
	return spec, describeSpecChange(before, spec), nil
}

// describeSpecChange renders the difference between two specs as one line, or an
// empty string when the roots are the same.
func describeSpecChange(before, after *pluginSpec) string {
	had := map[string]bool{}
	if before != nil {
		for _, r := range before.Roots {
			had[r.Name] = true
		}
	}
	has := map[string]bool{}
	for _, r := range after.Roots {
		has[r.Name] = true
	}
	var added, removed []string
	for name := range has {
		if !had[name] {
			added = append(added, "+"+name)
		}
	}
	for name := range had {
		if !has[name] {
			removed = append(removed, "-"+name)
		}
	}
	if before == nil && len(added) == 0 {
		return ""
	}
	changes := slices.Concat(added, removed)
	if len(changes) == 0 {
		return ""
	}
	sortStringsStable(changes)
	return "plugin commands updated: " + strings.Join(changes, " ")
}

// sortStringsStable orders the change list so the line is reproducible.
func sortStringsStable(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// skippedPluginKinds returns the commands this binary cannot execute, grouped by
// plugin, for `doctor` to report.
func skippedPluginKinds(spec *pluginSpec) map[string]int {
	if spec == nil {
		return nil
	}
	out := map[string]int{}
	for _, cmd := range spec.Commands {
		if !knownPluginKinds[cmd.Kind] {
			out[cmd.Slug]++
		}
	}
	return out
}

// maybeRefreshPluginSpec revalidates the cache at the three points where the
// operator has just told the CLI which server they mean: `login`, `context use`
// and `plugin cli refresh`.
//
// The source plan proposed piggybacking an If-None-Match onto the first request
// of every invocation instead. That does not work: the validator belongs to the
// spec endpoint, so revalidating it means a second request, and adding one to
// every command is a worse trade than a tree that lags until the next login. A
// pre-existing log-follow test caught exactly that extra request.
//
// Every error here is deliberately swallowed: a stale tree is a far smaller
// problem than a `login` that fails because the spec endpoint was slow.
func maybeRefreshPluginSpec(g *globals) {
	if pluginSpecDisabled() {
		return
	}
	cfg, err := LoadConfig()
	if err != nil {
		return
	}
	target, err := resolveTarget(cfg, g)
	if err != nil || target.URL == "" || target.APIKey == "" {
		return
	}
	cached := loadPluginSpec(target.ContextName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := newAPI(target)
	if err != nil {
		return
	}
	etag := ""
	if cached != nil {
		etag = cached.ETag
	}
	spec, err := fetchPluginSpec(ctx, a, etag)
	if err != nil || spec == nil || spec.SpecVersion != pluginSpecVersion {
		return
	}
	if err := savePluginSpec(target.ContextName, *spec); err != nil {
		return
	}
	if line := describeSpecChange(cached, spec); line != "" {
		emitln(g.stderr, line)
	}
}
