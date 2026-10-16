// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Plugin dependencies.
//
// A manifest may declare `requires: [<slug>, ...]`. The installer pulls each
// missing one in alongside the plugin that asked for it, and removes it again
// when the last plugin requiring it is uninstalled.
//
// The motivating case is the compute family: addon-kvm and addon-lxc both build
// on addon-compute's pools, networks, images and capability probe, and neither
// works without it. Making an operator install three plugins to get one working
// feature — and know which three — is the sort of thing that turns a working
// feature into a support question.
//
// # What this deliberately is not
//
// Not a version resolver. Slugs only, no constraints, no SAT solving. A plugin
// family that ships together moves together, and the machinery a real resolver
// needs would be several hundred lines serving a problem this host does not
// have. If that changes, this is the file to change.

// maxDependencyDepth bounds the recursion.
//
// Two is already generous — a dependency's own dependency — and the guard is
// really about a manifest that names itself or a pair that name each other. A
// cycle would otherwise install until the disk filled.
const maxDependencyDepth = 4

// installDependencies pulls in whatever the just-installed plugin requires.
//
// A dependency that fails to install is a warning, not an install failure. The
// plugin that asked for it is already on disk and its migrations have run;
// rolling it back over a dependency the operator can install by hand would turn
// a partial success into a total one. The plugin surfaces the missing
// dependency itself — addon-kvm without addon-compute renders a list that says
// no host can run guests — which is a better failure than no plugin at all.
func (d *DefaultInstaller) installDependencies(
	ctx context.Context,
	manifest *plugin.Manifest,
	progress ProgressFunc,
	depth int,
	allowUnsigned bool,
) {
	if manifest == nil || len(manifest.Requires) == 0 {
		return
	}
	if depth >= maxDependencyDepth {
		if d.logger != nil {
			d.logger.Warn("plugin install: dependency chain too deep, stopping",
				"slug", manifest.Slug, "depth", depth)
		}
		return
	}

	for _, required := range manifest.Requires {
		if required == "" || required == manifest.Slug {
			continue
		}

		installed, err := d.svc.LookupVersion(ctx, required)
		if err != nil {
			d.warn("plugin install: checking dependency", required, err)
			continue
		}
		if installed != "" {
			// Already there, however it got there. Nothing to do, and in
			// particular nothing to re-flag: a dependency the operator
			// installed deliberately stays theirs.
			continue
		}

		source, err := d.catalogSource(ctx, required)
		if err != nil {
			d.warn("plugin install: resolving dependency", required, err)
			continue
		}

		// A dependency inherits the operator's consent to the plugin that
		// named it; a signed install never silently pulls in an unsigned one.
		source.AllowUnsigned = allowUnsigned
		progress("dependency", 95, fmt.Sprintf("installing dependency %s", required))
		result, err := d.installDependency(ctx, source, depth+1)
		if err != nil {
			d.warn("plugin install: installing dependency", required, err)
			continue
		}

		// Flagged as the host's to clean up later. Set after the install
		// rather than during it so a dependency that half-installs leaves no
		// flag claiming the host owns something that is not there.
		if err := d.svc.MarkInstalledAsDependency(ctx, result.Slug); err != nil {
			d.warn("plugin install: flagging dependency", required, err)
		}

		if d.audit != nil {
			d.emitAudit(ctx, result.Slug, "plugin.install.dependency", map[string]any{
				"version":     result.Version,
				"required_by": manifest.Slug,
			})
		}
		if d.logger != nil {
			d.logger.Info("plugin install: pulled in dependency",
				"slug", result.Slug, "version", result.Version, "required_by", manifest.Slug)
		}
	}
}

// installDependency installs one dependency and recurses into its own.
//
// Separate from Install so the recursion carries the depth. Install itself
// enters at depth zero.
func (d *DefaultInstaller) installDependency(
	ctx context.Context,
	src Source,
	depth int,
) (*Installed, error) {
	result, err := d.install(ctx, src, nil, depth)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// pruneDependencies removes dependencies the uninstalled plugin was the last
// to need.
//
// Three conditions, all required, and each one is a way this could otherwise go
// wrong:
//
//   - The dependency must be flagged `installed_as_dependency`. An operator who
//     installed Compute Foundation on purpose, then tried the KVM plugin and
//     removed it, keeps what they chose.
//   - No other installed plugin may still require it. Removing addon-compute
//     when addon-kvm goes would break addon-lxc, which is exactly the case the
//     user asked about.
//   - A failure is logged, not returned. The plugin the operator asked to
//     remove is already gone; failing the uninstall over a leftover dependency
//     would report a failure for work that succeeded.
func (d *DefaultInstaller) pruneDependencies(
	ctx context.Context,
	removed *plugin.Manifest,
	opts UninstallOptions,
	progress ProgressFunc,
	depth int,
) {
	if removed == nil || len(removed.Requires) == 0 {
		return
	}
	if depth >= maxDependencyDepth {
		return
	}

	for _, candidate := range removed.Requires {
		if candidate == "" || candidate == removed.Slug {
			continue
		}

		byDependency, err := d.svc.InstalledAsDependency(ctx, candidate)
		if err != nil {
			d.warn("plugin uninstall: checking dependency flag", candidate, err)
			continue
		}
		if !byDependency {
			continue
		}

		dependents, err := d.dependentsOf(ctx, candidate)
		if err != nil {
			d.warn("plugin uninstall: finding dependents", candidate, err)
			continue
		}
		if len(dependents) > 0 {
			if d.logger != nil {
				d.logger.Info("plugin uninstall: dependency kept, still required",
					"slug", candidate, "required_by", dependents)
			}
			continue
		}

		progress("dependency", 95, fmt.Sprintf("removing unused dependency %s", candidate))
		if _, err := d.uninstall(ctx, candidate, opts, nil, depth+1); err != nil {
			d.warn("plugin uninstall: removing dependency", candidate, err)
			continue
		}

		if d.audit != nil {
			d.emitAudit(ctx, candidate, "plugin.uninstall.dependency", map[string]any{
				"released_by": removed.Slug,
			})
		}
		if d.logger != nil {
			d.logger.Info("plugin uninstall: removed unused dependency",
				"slug", candidate, "released_by", removed.Slug)
		}
	}
}

// dependentsOf names the installed plugins that still require a slug.
//
// Read from each installed plugin's manifest on disk rather than from a
// dependency table. The manifest is the authority and cannot drift from what is
// actually installed; a table would be a second copy to keep in step, and the
// number of installed plugins is small enough that reading them is free.
//
// A manifest that cannot be read counts as a dependent. That is the safe
// direction: keeping a dependency nothing needs wastes disk, and removing one
// something needs breaks a working plugin.
func (d *DefaultInstaller) dependentsOf(ctx context.Context, slug string) ([]string, error) {
	installed, err := d.svc.ListInstalled(ctx)
	if err != nil {
		return nil, err
	}

	var dependents []string
	for _, entry := range installed {
		if entry.Slug == slug {
			continue
		}
		manifest, err := d.readInstalledManifest(entry.Slug, entry.Version)
		if err != nil {
			d.warn("plugin uninstall: reading manifest, assuming it depends", entry.Slug, err)
			dependents = append(dependents, entry.Slug)
			continue
		}
		for _, required := range manifest.Requires {
			if required == slug {
				dependents = append(dependents, entry.Slug)
				break
			}
		}
	}
	return dependents, nil
}

// readInstalledManifest reads one installed plugin's manifest from disk.
func (d *DefaultInstaller) readInstalledManifest(slug, version string) (*plugin.Manifest, error) {
	path := filepath.Join(d.dataDir, "plugins", slug+"@"+version, "plugin.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s manifest: %w", slug, err)
	}
	var manifest plugin.Manifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parsing %s manifest: %w", slug, err)
	}
	return &manifest, nil
}

// catalogSource finds the download for a slug in the configured catalogs.
func (d *DefaultInstaller) catalogSource(ctx context.Context, slug string) (Source, error) {
	entries, err := d.svc.ListCatalog(ctx)
	if err != nil {
		return Source{}, err
	}
	for _, entry := range entries {
		if entry.Slug != slug {
			continue
		}
		// Versions is sorted newest first; the top-level fields mirror it for
		// the legacy single-version shape.
		if len(entry.Versions) > 0 {
			latest := entry.Versions[0]
			return Source{URL: latest.DownloadURL, Checksum: latest.Checksum, Build: latest.Build, BuiltFor: latest.BuiltFor}, nil
		}
		if entry.DownloadURL == "" {
			return Source{}, fmt.Errorf("catalog entry %s has no download", slug)
		}
		return Source{URL: entry.DownloadURL, Checksum: entry.Checksum, Build: entry.Build, BuiltFor: entry.BuiltFor}, nil
	}
	return Source{}, fmt.Errorf("%s is not in any configured plugin catalog", slug)
}

func (d *DefaultInstaller) warn(message, slug string, err error) {
	if d.logger != nil {
		d.logger.Warn(message, "slug", slug, "error", err)
	}
}
