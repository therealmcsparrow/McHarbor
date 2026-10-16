// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// pluginSlugPrefixes are the families a plugin slug must start with, matching
// VALID_SLUG_PREFIXES in scripts/development/build-plugin.sh. A slug outside
// them is refused by the packager, so it is refused here rather than three
// minutes into a Docker build.
var pluginSlugPrefixes = []string{
	"addon-", "ai-provider-", "comm-", "identity-",
	"logging-", "scanner-", "snapshot-", "storage-",
}

// pluginDevEnv is where a plugin authoring command does its work: the checkout,
// and how it reaches the shell scripts that already know how to build one.
type pluginDevEnv struct {
	repoRoot string
	dryRun   bool
	g        *globals
}

// addPluginDevCommands mounts the authoring toolchain under the existing plugin
// group as `plugin dev`. It is deliberately a subgroup rather than a second root:
// an author reaching for it already thinks "plugin", and the help text on both
// levels says which half talks to a server and which half does not.
func addPluginDevCommands(root *cobra.Command, g *globals) {
	dev := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"author"},
		Short:   "Author a plugin locally: scaffold, build, package, sign and validate",
		Long: "plugin dev is the plugin author's local toolchain. Unlike every other mcharbor command it " +
			"talks to no server: it reads and writes your checkout and drives the build scripts and the " +
			"signer. There is no API key, no context and no permission key involved, and nothing it does " +
			"is audited, because nothing it does reaches a McHarbor instance.\n\n" +
			"The sibling commands — plugin list, catalog, install, update, uninstall — are the runtime half, " +
			"and those do talk to a server and do need plugins.view / plugins.manage.",
	}

	env := &pluginDevEnv{g: g}
	dev.PersistentFlags().StringVar(&env.repoRoot, "repo", "", "McHarbor checkout to work in; found from the working directory when omitted ($MCHARBOR_REPO_ROOT)")
	dev.PersistentFlags().BoolVar(&env.dryRun, "dry-run", false, "print the command that would run instead of running it")

	dev.AddCommand(pluginDevNew(env))
	dev.AddCommand(pluginDevValidate(env))
	dev.AddCommand(pluginDevBuild(env))
	dev.AddCommand(pluginDevPackage(env))
	dev.AddCommand(pluginDevSign(env))

	if parent := findCommand(root, "plugin"); parent != nil {
		parent.AddCommand(dev)
		return
	}
	// The runtime plugin group is registered by addPlatformCommands. Standing on
	// our own is better than losing the group if that call order ever changes.
	standalone := &cobra.Command{Use: "plugin", Short: "Plugin authoring"}
	standalone.AddCommand(dev)
	root.AddCommand(standalone)
}

// findCommand returns root's direct subcommand called name, or nil.
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, sub := range root.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

// root resolves the checkout once: the --repo flag, then MCHARBOR_REPO_ROOT,
// then the nearest ancestor of the working directory holding src/plugins/Plugins.
func (e *pluginDevEnv) root() (string, error) {
	if e.repoRoot != "" {
		return e.repoRoot, nil
	}
	if fromEnv := os.Getenv("MCHARBOR_REPO_ROOT"); fromEnv != "" {
		e.repoRoot = fromEnv
		return e.repoRoot, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", usageError("cannot read the working directory: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "src", "plugins", "Plugins")); statErr == nil && info.IsDir() {
			e.repoRoot = dir
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", usageError("no McHarbor checkout found above %s; pass --repo or set MCHARBOR_REPO_ROOT", dir)
		}
		dir = parent
	}
}

// slugs lists every plugin directory in the checkout. There are no template
// directories left to filter out: the scaffold template lives in this binary.
func (e *pluginDevEnv) slugs() ([]string, error) {
	repo, err := e.root()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(repo, "src", "plugins", "Plugins"))
	if err != nil {
		return nil, usageError("cannot list the plugin tree: %v", err)
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// run executes one of the repository's own scripts. The scripts are POSIX shell,
// so they go through bash, which is what a Windows checkout has through Git.
func (e *pluginDevEnv) run(ctx context.Context, what string, script string, args ...string) error {
	repo, err := e.root()
	if err != nil {
		return err
	}
	path := filepath.Join(repo, script)
	if _, err := os.Stat(path); err != nil {
		return usageError("%s not found in %s; is this a full checkout?", script, repo)
	}
	full := append([]string{toSlashPath(path)}, args...)
	if e.dryRun {
		emitf(e.g.stdout, "bash %s\n", strings.Join(full, " "))
		return nil
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		return usageError("bash is not on PATH; %s is a shell script. On Windows, use the Git Bash that ships with Git", script)
	}
	emitf(e.g.stderr, "==> %s\n", what)
	cmd := exec.CommandContext(ctx, bash, full...)
	cmd.Dir = repo
	cmd.Stdout = e.g.stdout
	cmd.Stderr = e.g.stderr
	cmd.Stdin = stdin
	if err := cmd.Run(); err != nil {
		return &Error{Code: ExitFailure, Message: what + " failed: " + err.Error()}
	}
	return nil
}

// toSlashPath hands bash a path it can open on either platform.
func toSlashPath(p string) string { return filepath.ToSlash(p) }

func pluginDevNew(e *pluginDevEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new plugin from the template built into this binary",
		Long: "new writes a minimal working plugin into src/plugins/Plugins/<slug> and rewrites the slug in " +
			"every file that names it: the manifest, the Go module path, the exported Version symbol, the " +
			"frontend bundle name and the npm package name. The template is embedded in this binary, so it " +
			"cannot drift out of the checkout.\n\n" +
			"Start a plugin this way rather than by copying a sibling. A sibling carries its vendor quirks, " +
			"its bugs and whatever pattern was current when it was written, which is how one client " +
			"implementation reached fourteen plugins.\n\n" +
			"It refuses to overwrite an existing plugin. The slug must start with one of the families the " +
			"packager accepts: " + strings.Join(pluginSlugPrefixes, ", ") + ".",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return e.scaffold(args[0])
		},
	}
}

// scaffold writes the embedded template into a new plugin directory, rewriting
// the placeholder slug wherever it appears. Nothing is written until the target
// is known absent, so a mistyped slug cannot half-create a plugin.
func (e *pluginDevEnv) scaffold(slug string) error {
	repo, err := e.root()
	if err != nil {
		return err
	}
	if err := checkPluginSlug(slug); err != nil {
		return err
	}
	files, err := pluginTemplateFiles()
	if err != nil {
		return &Error{Code: ExitFailure, Message: "reading the embedded plugin template: " + err.Error()}
	}
	if len(files) == 0 {
		return &Error{Code: ExitFailure,
			Message: "the embedded plugin template is empty; this binary was built without templates/plugin"}
	}

	dest := filepath.Join(repo, "src", "plugins", "Plugins", slug)
	if _, err := os.Stat(dest); err == nil {
		return usageError("%s already exists; pick another slug or delete it first", dest)
	}

	for rel, body := range files {
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return &Error{Code: ExitFailure, Message: "scaffolding " + slug + ": " + err.Error()}
		}
		if err := os.WriteFile(target, renderPluginTemplate(body, slug), 0o644); err != nil {
			return &Error{Code: ExitFailure, Message: "scaffolding " + slug + ": " + err.Error()}
		}
	}

	emitf(e.g.stderr, "Wrote %d file(s) to %s.\n", len(files), dest)
	emitf(e.g.stdout, "%s scaffolded\n", slug)
	emitf(e.g.stderr,
		"Next: add it to scripts/development/plugins.list, then mcharbor plugin dev validate %s.\n", slug)
	return nil
}

func checkPluginSlug(slug string) error {
	for _, prefix := range pluginSlugPrefixes {
		if strings.HasPrefix(slug, prefix) && len(slug) > len(prefix) {
			return nil
		}
	}
	return usageError("slug %q must start with one of: %s", slug, strings.Join(pluginSlugPrefixes, " "))
}

func pluginDevValidate(e *pluginDevEnv) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [slug...]",
		Short: "Check a plugin's manifest, its route declarations and its build inputs",
		Long: "validate reads the source tree and reports what would go wrong once the plugin is installed. " +
			"The check worth running it for is route drift: a plugin that registers a non-public route its " +
			"plugin.yaml never declares gets no permission middleware at all, because the loader returns an " +
			"empty permission on a miss and an empty permission is a pass-through — so the route is reachable " +
			"by any authenticated user whatever their role. It also refuses a CRLF go.mod, a dependency " +
			"version that disagrees with the host's, a permission key the installer will reject, a Version " +
			"symbol that has drifted from the manifest, and a UMD bundle older than its sources.\n\n" +
			"With no slug it validates every plugin in the checkout. It exits 1 when any check is an error " +
			"and 0 when the worst is a warning.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := args
			if len(targets) == 0 {
				all, err := e.slugs()
				if err != nil {
					return err
				}
				targets = all
			}
			repo, err := e.root()
			if err != nil {
				return err
			}
			p, err := e.g.printer()
			if err != nil {
				return err
			}
			var findings []pluginFinding
			for _, slug := range targets {
				found, err := validatePlugin(repo, slug)
				if err != nil {
					return err
				}
				findings = append(findings, found...)
			}
			rows := make([]map[string]any, 0, len(findings))
			for _, f := range findings {
				rows = append(rows, map[string]any{
					"slug": f.Slug, "severity": f.Severity, "check": f.Check, "detail": f.Detail,
				})
			}
			if err := p.List(rows, []Column{
				{Header: "PLUGIN", Path: "slug"}, {Header: "SEVERITY", Path: "severity"},
				{Header: "CHECK", Path: "check"}, {Header: "DETAIL", Path: "detail"},
			}); err != nil {
				return err
			}
			errors := 0
			for _, f := range findings {
				if f.Severity == "error" {
					errors++
				}
			}
			emitf(e.g.stderr, "%d plugin(s) checked, %d finding(s), %d error(s).\n",
				len(targets), len(findings), errors)
			if errors > 0 {
				return &Error{Code: ExitFailure, Operation: "pluginDevValidate",
					Message: fmt.Sprintf("%d error(s) in %d plugin(s); see the table above", errors, len(targets))}
			}
			return nil
		},
	}
}

func pluginDevBuild(e *pluginDevEnv) *cobra.Command {
	var all, stale, keepGoing bool
	var catalog string
	cmd := &cobra.Command{
		Use:   "build [slug...]",
		Short: "Build, sign and index a plugin into the local catalog",
		Long: "build drives scripts/development/catalog.sh, which rebuilds the frontend UMD bundle, compiles " +
			"the .so inside the host's Go module context through the packager stage, signs the tarball and " +
			"regenerates index.json. With no slug it builds what the last sweep left stale.\n\n" +
			"Editing anything under src/backend/core/plugin/** invalidates every .so, which no stamp can " +
			"tell you: use --all after that. A built plugin reaches a running instance only after " +
			"docker compose up -d --force-recreate mcharbor.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			script := []string{"build"}
			switch {
			case len(args) > 0:
				script = append(script, args...)
			case all:
				script = append(script, "--all")
			case stale:
				script = append(script, "--stale")
			}
			if keepGoing {
				script = append(script, "--keep-going")
			}
			if catalog != "" {
				script = append(script, "--catalog", catalog)
			}
			return e.run(cmd.Context(), "catalog.sh "+strings.Join(script, " "), "scripts/development/catalog.sh", script...)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "build every plugin in plugins.list; the only safe choice after editing core/plugin/**")
	cmd.Flags().BoolVar(&stale, "stale", false, "build only what the last sweep left behind (the default)")
	cmd.Flags().BoolVar(&keepGoing, "keep-going", false, "do not stop after the first failure")
	cmd.Flags().StringVar(&catalog, "catalog", "", "catalog URL baked into each entry's download_url")
	return cmd
}

func pluginDevPackage(e *pluginDevEnv) *cobra.Command {
	var catalog string
	cmd := &cobra.Command{
		Use:   "package <slug>",
		Short: "Package and sign one plugin without touching the catalog index",
		Long: "package drives scripts/development/build-plugin.sh for one plugin: the frontend bundle, the " +
			"packager stage, the signature and the catalog entry, but not index.json. Use it while iterating " +
			"on one plugin and plugin dev build when the catalog has to be consistent again.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := catalog
			if url == "" {
				url = "file:///mnt/plugin-catalog"
			}
			return e.run(cmd.Context(), "build-plugin.sh "+args[0], "scripts/development/build-plugin.sh", args[0], url)
		},
	}
	cmd.Flags().StringVar(&catalog, "catalog", "", "catalog URL baked into the entry's download_url (default file:///mnt/plugin-catalog)")
	return cmd
}

func pluginDevSign(e *pluginDevEnv) *cobra.Command {
	var keyPath, trustPath, dir string
	cmd := &cobra.Command{
		Use:   "sign [slug]",
		Short: "Sign the built tarballs in the local catalog directory",
		Long: "sign runs the host's own signer over a directory of tarballs and catalog entries, so the " +
			"signature the store verifies and the signature written here cannot differ. It prefers the " +
			"prebuilt src/backend/pluginsigner.exe, falls back to go run ./cmd/pluginsigner, and only then " +
			"to src/plugins/scripts/sign-plugin.py, which needs PyNaCl. The slug is accepted for symmetry " +
			"with the other verbs and narrows nothing: the signer works on a directory.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := e.root()
			if err != nil {
				return err
			}
			target := dir
			if target == "" {
				target = filepath.Join(repo, "mnt", "plugin-catalog")
			}
			key := keyPath
			if key == "" {
				key = filepath.Join(repo, "signing", "plugin-signing.key")
			}
			trust := trustPath
			if trust == "" {
				trust = filepath.Join(repo, "signing", "plugin-signing.pub")
			}
			if _, err := os.Stat(key); err != nil {
				return usageError("no signing key at %s; pass --key", key)
			}
			return e.sign(cmd.Context(), repo, target, key, trust)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory holding the tarballs and catalog entries (default <repo>/mnt/plugin-catalog)")
	cmd.Flags().StringVar(&keyPath, "key", "", "ed25519 private key (default <repo>/signing/plugin-signing.key)")
	cmd.Flags().StringVar(&trustPath, "trust", "", "ed25519 public key to verify each signature against")
	return cmd
}

// sign picks the strongest available signer. The prebuilt binary and go run are
// the same code the image build uses; the Python script is a fallback for a
// checkout with no Go toolchain and signs one tarball rather than a directory.
func (e *pluginDevEnv) sign(ctx context.Context, repo, dir, key, trust string) error {
	args := []string{"-dir", toSlashPath(dir), "-key", toSlashPath(key)}
	if _, err := os.Stat(trust); err == nil {
		args = append(args, "-trust", toSlashPath(trust))
	}

	signer := filepath.Join(repo, "src", "backend", "pluginsigner.exe")
	if _, err := os.Stat(signer); err == nil {
		return e.exec(ctx, "pluginsigner.exe", filepath.Join(repo, "src", "backend"), nil, signer, args...)
	}
	if _, err := exec.LookPath("go"); err == nil {
		goArgs := append([]string{"run", "./cmd/pluginsigner"}, args...)
		return e.exec(ctx, "go run ./cmd/pluginsigner", filepath.Join(repo, "src", "backend"),
			[]string{"GOTOOLCHAIN=auto", "GOWORK=off"}, "go", goArgs...)
	}
	python, err := exec.LookPath("python")
	if err != nil {
		return usageError("no signer available: %s is absent, go is not on PATH, and neither is python", signer)
	}
	emitln(e.g.stderr,
		"No pluginsigner binary and no Go toolchain; falling back to sign-plugin.py, which needs PyNaCl and signs one tarball at a time.")
	tarballs, err := filepath.Glob(filepath.Join(dir, "files", "*.tar.gz"))
	if err != nil || len(tarballs) == 0 {
		return usageError("no tarballs under %s", filepath.Join(dir, "files"))
	}
	script := filepath.Join(repo, "src", "plugins", "scripts", "sign-plugin.py")
	for _, tarball := range tarballs {
		if err := e.exec(ctx, "sign-plugin.py "+filepath.Base(tarball), repo, nil, python,
			toSlashPath(script), "--tarball", toSlashPath(tarball), "--key", toSlashPath(key)); err != nil {
			return err
		}
	}
	return nil
}

// exec runs one external program, honouring --dry-run the same way run does.
func (e *pluginDevEnv) exec(ctx context.Context, what, workDir string, extraEnv []string, name string, args ...string) error {
	if e.dryRun {
		emitf(e.g.stdout, "%s %s\n", name, strings.Join(args, " "))
		return nil
	}
	emitf(e.g.stderr, "==> %s\n", what)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = e.g.stdout
	cmd.Stderr = e.g.stderr
	if err := cmd.Run(); err != nil {
		return &Error{Code: ExitFailure, Message: what + " failed: " + err.Error()}
	}
	return nil
}
