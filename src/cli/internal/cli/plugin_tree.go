// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// addPluginCommands grafts the cached plugin command spec onto the tree.
//
// It reads the cache only and never touches the network, because it runs on
// every invocation including `--help`: a tree that needed a request would make
// the CLI unusable offline and slow when it was not.
func addPluginCommands(root *cobra.Command, g *globals) {
	if pluginSpecDisabled() {
		return
	}
	cfg, err := LoadConfig()
	if err != nil {
		return
	}
	spec := loadPluginSpec(resolveContextName(cfg, g))
	if spec == nil {
		return
	}
	reserved := coreCommandNames(root)
	for _, pr := range spec.Roots {
		// A root that would shadow a core command is dropped here as well as
		// server-side: this binary's own tree is the authority on what its core
		// commands are called, and it may be newer than the server.
		if reserved[pr.Name] {
			continue
		}
		group := &cobra.Command{
			Use:     pr.Name,
			Short:   pluginShort(pr),
			Aliases: usableAliases(pr.Aliases, reserved),
		}
		attached := 0
		for _, cmd := range spec.Commands {
			if cmd.Slug != pr.Slug || !knownPluginKinds[cmd.Kind] {
				continue
			}
			if attachPluginCommand(group, g, cmd) {
				attached++
			}
		}
		if attached == 0 {
			continue
		}
		root.AddCommand(group)
	}
}

// pluginShort falls back to something honest when a plugin declared no summary.
func pluginShort(pr pluginRoot) string {
	if strings.TrimSpace(pr.Short) != "" {
		return pr.Short
	}
	return "Commands contributed by " + pr.Slug
}

// usableAliases drops an alias that would shadow a core command.
func usableAliases(aliases []string, reserved map[string]bool) []string {
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if a != "" && !reserved[a] {
			out = append(out, a)
		}
	}
	return out
}

// coreCommandNames collects the top-level command names and aliases this binary
// ships, which is what a plugin root must not collide with.
func coreCommandNames(root *cobra.Command) map[string]bool {
	out := map[string]bool{"help": true, "completion": true}
	for _, c := range root.Commands() {
		out[c.Name()] = true
		for _, a := range c.Aliases {
			out[a] = true
		}
	}
	return out
}

// attachPluginCommand builds one cobra command and hangs it at its declared
// path, creating the intermediate groups as it goes. It reports false when the
// declaration cannot be turned into a usable command.
func attachPluginCommand(group *cobra.Command, g *globals, cmd pluginCLICommand) bool {
	if len(cmd.Path) == 0 {
		return false
	}
	parent := group
	for _, segment := range cmd.Path[:len(cmd.Path)-1] {
		parent = ensureGroup(parent, segment)
	}
	leaf := cmd.Path[len(cmd.Path)-1]
	if existing := findChild(parent, leaf); existing != nil {
		// Two declarations at one path. The server's unique index makes this
		// impossible per plugin, so reaching it means a stale cache; the first
		// wins rather than cobra panicking on a duplicate.
		return false
	}

	exec := newPluginExecutor(g, cmd)
	sub := &cobra.Command{
		Use:     usageLine(leaf, cmd),
		Short:   cmd.Short,
		Long:    pluginLong(cmd),
		Example: cmd.Example,
		Args:    cobra.ExactArgs(len(cmd.Args)),
		RunE:    exec.run,
	}
	exec.bind(sub)
	parent.AddCommand(sub)
	return true
}

// usageLine renders the positional arguments into the usage string, so `--help`
// shows `snapshot create <id>` rather than leaving the operator to guess.
func usageLine(leaf string, cmd pluginCLICommand) string {
	if len(cmd.Args) == 0 {
		return leaf
	}
	parts := make([]string, 0, len(cmd.Args)+1)
	parts = append(parts, leaf)
	for _, a := range cmd.Args {
		parts = append(parts, "<"+a+">")
	}
	return strings.Join(parts, " ")
}

// pluginLong appends the permission and the owning plugin to a command's long
// help, because a plugin command's provenance and its key are the two things an
// operator asks about when it is refused.
func pluginLong(cmd pluginCLICommand) string {
	var b strings.Builder
	if strings.TrimSpace(cmd.Long) != "" {
		b.WriteString(strings.TrimSpace(cmd.Long))
		b.WriteString("\n\n")
	}
	b.WriteString("Contributed by the " + cmd.Slug + " plugin.")
	if cmd.Needs != "" {
		b.WriteString(" It needs " + cmd.Needs + ".")
	}
	if cmd.Risk == "irreversible" {
		b.WriteString(" It cannot be undone, so --yes is not enough: pass --confirm <target>.")
	}
	return b.String()
}

// ensureGroup finds or creates an intermediate command group.
func ensureGroup(parent *cobra.Command, name string) *cobra.Command {
	if existing := findChild(parent, name); existing != nil {
		return existing
	}
	group := &cobra.Command{Use: name, Short: name + " commands"}
	parent.AddCommand(group)
	return group
}

// findChild returns parent's child called name, or nil.
func findChild(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// pluginTreeSummary renders the cached tree for `plugin cli show`, sorted so the
// output is reproducible.
func pluginTreeSummary(spec *pluginSpec) []map[string]any {
	if spec == nil {
		return nil
	}
	byRoot := map[string][]pluginCLICommand{}
	rootName := map[string]string{}
	for _, r := range spec.Roots {
		rootName[r.Slug] = r.Name
	}
	for _, c := range spec.Commands {
		byRoot[rootName[c.Slug]] = append(byRoot[rootName[c.Slug]], c)
	}
	names := make([]string, 0, len(byRoot))
	for name := range byRoot {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(spec.Commands))
	for _, name := range names {
		commands := byRoot[name]
		sort.Slice(commands, func(i, j int) bool {
			return strings.Join(commands[i].Path, " ") < strings.Join(commands[j].Path, " ")
		})
		for _, c := range commands {
			out = append(out, map[string]any{
				"command": "mcharbor " + name + " " + strings.Join(c.Path, " "),
				"plugin":  c.Slug,
				"kind":    c.Kind,
				"needs":   c.Needs,
				"risk":    c.Risk,
			})
		}
	}
	return out
}

// addPluginCLICommands adds `plugin cli list|refresh|explain`, which are how an
// operator inspects a tree they did not write.
func addPluginCLICommands(parent *cobra.Command, g *globals) {
	cli := &cobra.Command{Use: "cli", Short: "The plugin-contributed command tree this context offers"}

	cli.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls", "show"}, Short: "List the cached plugin commands and the cache's age",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			cfg, err := g.config()
			if err != nil {
				return err
			}
			spec := loadPluginSpec(resolveContextName(cfg, g))
			if spec == nil {
				if pluginSpecDisabled() {
					emitln(g.stderr, "MCHARBOR_NO_PLUGIN_CLI=1 is set, so no plugin commands are built.")
					return nil
				}
				emitln(g.stderr, "No plugin command spec cached for this context yet; run `mcharbor plugin cli refresh`.")
				return nil
			}
			rows := pluginTreeSummary(spec)
			if err := p.List(rows, []Column{
				{Header: "COMMAND", Path: "command"}, {Header: "PLUGIN", Path: "plugin"},
				{Header: "KIND", Path: "kind"}, {Header: "NEEDS", Path: "needs"}, {Header: "RISK", Path: "risk"},
			}); err != nil {
				return err
			}
			emitf(g.stderr, "%d command(s) from %d plugin(s), fetched %s from server build %s\n",
				len(rows), len(spec.Roots), age(spec.FetchedAt), spec.Build)
			for slug, n := range skippedPluginKinds(spec) {
				emitf(g.stderr, "%d command(s) from %s need a newer CLI\n", n, slug)
			}
			return nil
		},
	})

	cli.AddCommand(&cobra.Command{
		Use: "refresh", Short: "Fetch the plugin command spec now", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := g.ctx(cmd)
			defer cancel()
			spec, changed, err := refreshPluginSpec(ctx, g)
			if err != nil {
				return err
			}
			if spec == nil {
				emitln(g.stderr, "The server offers no plugin commands.")
				return nil
			}
			if changed != "" {
				emitln(g.stderr, changed)
			}
			emitf(g.stdout, "%d command(s) from %d plugin(s), server build %s.\n",
				len(spec.Commands), len(spec.Roots), spec.Build)
			return nil
		},
	})

	cli.AddCommand(&cobra.Command{
		Use: "explain <root> [path...]", Short: "Show what one plugin command does and which route it calls",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			cfg, err := g.config()
			if err != nil {
				return err
			}
			spec := loadPluginSpec(resolveContextName(cfg, g))
			if spec == nil {
				return usageError("no plugin command spec cached for this context; run `mcharbor plugin cli refresh`")
			}
			wantRoot, wantPath := args[0], strings.Join(args[1:], " ")
			for _, r := range spec.Roots {
				if r.Name != wantRoot {
					continue
				}
				for _, c := range spec.Commands {
					if c.Slug != r.Slug {
						continue
					}
					if wantPath != "" && strings.Join(c.Path, " ") != wantPath {
						continue
					}
					return p.Object(map[string]any{
						"command": "mcharbor " + r.Name + " " + strings.Join(c.Path, " "),
						"plugin":  c.Slug, "kind": c.Kind, "method": c.Method, "route": c.Route,
						"needs": c.Needs, "risk": c.Risk, "envScoped": c.EnvScoped,
						"args": c.Args, "flags": c.Flags, "columns": c.Columns,
					}, nil)
				}
			}
			return &Error{Code: ExitNotFound, Message: fmt.Sprintf("no plugin command %q in the cached spec", strings.TrimSpace(wantRoot+" "+wantPath))}
		},
	})

	parent.AddCommand(cli)
}

// pluginSpecDoctorLines is what `doctor` reports about the plugin surface: the
// cache's age and source, the commands this binary cannot run, and a root whose
// plugin has since been uninstalled.
func pluginSpecDoctorLines(g *globals) []string {
	if pluginSpecDisabled() {
		return []string{"plugin commands: disabled by MCHARBOR_NO_PLUGIN_CLI"}
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil
	}
	contextName := resolveContextName(cfg, g)
	spec := loadPluginSpec(contextName)
	if spec == nil {
		return []string{"plugin commands: none cached for this context"}
	}
	lines := []string{fmt.Sprintf("plugin commands: %d from %d plugin(s), fetched %s from server build %s",
		len(spec.Commands), len(spec.Roots), age(spec.FetchedAt), spec.Build)}
	for slug, n := range skippedPluginKinds(spec) {
		lines = append(lines, fmt.Sprintf("plugin commands: %d from %s need a newer CLI", n, slug))
	}
	// A command whose root is gone is a stale cache, which is the one state that
	// makes the tree lie about what the server offers.
	roots := map[string]bool{}
	for _, r := range spec.Roots {
		roots[r.Slug] = true
	}
	orphans := map[string]bool{}
	for _, c := range spec.Commands {
		if !roots[c.Slug] {
			orphans[c.Slug] = true
		}
	}
	for slug := range orphans {
		lines = append(lines, "plugin commands: "+slug+" has commands but no root; run `mcharbor plugin cli refresh`")
	}
	if contextName != "" {
		lines = append(lines, "plugin commands: cached for context "+contextName)
	}
	sort.Strings(lines[1:])
	return lines
}
