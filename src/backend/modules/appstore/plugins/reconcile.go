// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Reconcile reinstalls every plugin whose installed artifact no longer matches
// the catalog, because anything that changes a package the plugins link
// invalidates the ABI of every .so on disk.
//
// Two things trigger it. A changed built_for is the obvious one: a core upgrade.
// A changed build stamp at the same version is the one that matters more, and
// the reason builds exist — a host rebuilt from edited source keeps its version,
// so built_for still matches while every .so has become unloadable. Without the
// build there is nothing left to compare and the operator has to reinstall each
// plugin by hand.
//
// Nothing here is fatal. Plugins no longer ship inside the image, so a host that
// refused to boot over an unreachable catalog would be unrecoverable offline —
// a missing sidebar section is the far cheaper failure.
func (h *Handler) Reconcile(ctx context.Context, logger Logger) {
	current := appversion.Current()

	// First, so a plugin an interrupted upgrade left half-done is whole again
	// before its build stamp is compared.
	h.RecoverInterruptedUpgrades(ctx, logger)

	installed, err := h.svc.ListInstalled(ctx)
	if err != nil {
		if logger != nil {
			logger.Error("plugins: reconcile could not list installs", "error", err.Error())
		}
		return
	}

	// Read once rather than per plugin: catalogSourceFor rescans the whole
	// catalog on every call, and this loop already pays that in repairOne.
	catalog, err := h.svc.ListCatalog(ctx)
	if err != nil && logger != nil {
		logger.Warn("plugins: reconcile could not read the catalog; "+
			"falling back to built_for alone", "error", err.Error())
	}

	for _, p := range installed {
		staleRelease := p.BuiltFor != current
		catalogBuild := buildFor(catalog, p.Slug, p.Version)
		staleBuild := catalogBuild != "" && catalogBuild != p.Build
		if !staleRelease && !staleBuild {
			continue
		}
		// A repair reinstalls the version the operator already accepted, so an
		// unsigned install stays repairable; one that was signed must still be.
		if err := h.repairOne(ctx, p, !p.Signed, staleRelease); err != nil {
			if logger != nil {
				logger.Error("plugins: repair failed", "slug", p.Slug,
					"built_for", p.BuiltFor, "host", current, "error", err.Error())
			}
			h.keepOrDisable(ctx, p, err, logger)
			continue
		}
		if logger != nil {
			logger.Info("plugins: repaired plugin", "slug", p.Slug,
				"was", p.BuiltFor, "now", current,
				"wasBuild", p.Build, "nowBuild", catalogBuild)
		}
	}
}

// buildFor returns the catalog's build stamp for one slug at one version, and
// empty when the catalog does not carry that version or predates build stamps.
//
// Empty is deliberately never treated as a mismatch: a catalog with no stamps
// would otherwise reinstall every plugin on every boot.
func buildFor(catalog []PluginEntry, slug, version string) string {
	for _, entry := range catalog {
		if entry.Slug != slug {
			continue
		}
		for _, v := range entry.Versions {
			if v.Version == version {
				return v.Build
			}
		}
		return ""
	}
	return ""
}

// repairOne reinstalls a single plugin at the version already recorded for it,
// so a repair never silently upgrades the operator to a newer release.
//
// It goes through upgrade: the catalog artifact is resolved, verified and
// validated before anything is torn down, the role grants are persisted before
// the uninstall strips them, and a failed install reinstates the previous copy.
// Signature consent is enforced by the preflight itself.
func (h *Handler) repairOne(ctx context.Context, p InstalledPlugin, allowUnsigned, releaseChanged bool) error {
	src, err := h.catalogSourceFor(ctx, p.Slug, p.Version, releaseChanged)
	if err != nil {
		return err
	}
	src.AllowUnsigned = allowUnsigned
	if _, err := h.upgrade(ctx, p.Slug, src, nil); err != nil {
		return err
	}
	if err := h.svc.SetRepairState(ctx, p.Slug, appversion.Current(), ""); err != nil {
		return err
	}
	h.notifyChange(p.Slug)
	return nil
}

// catalogSourceFor finds the artifact for one slug at one version, preferring an
// exact version match so a repair restores what was installed rather than the
// newest release the catalog happens to carry. When the host release changed and
// the catalog no longer carries that version, see repairVersion.
func (h *Handler) catalogSourceFor(ctx context.Context, slug, version string, releaseChanged bool) (Source, error) {
	entries, err := h.svc.ListCatalog(ctx)
	if err != nil {
		return Source{}, fmt.Errorf("reading plugin catalog: %w", err)
	}
	for _, entry := range entries {
		if entry.Slug != slug {
			continue
		}
		v, ok := repairVersion(entry.Versions, version, appversion.Current(), releaseChanged)
		if ok {
			if strings.TrimSpace(v.DownloadURL) == "" {
				return Source{}, fmt.Errorf("catalog entry %s@%s has no download", slug, v.Version)
			}
			return Source{
				URL:       v.DownloadURL,
				Checksum:  v.Checksum,
				Signature: v.Signature,
				Build:     v.Build,
				BuiltFor:  v.BuiltFor,
			}, nil
		}
		return Source{}, fmt.Errorf("catalog has no %s@%s built for %s",
			slug, version, appversion.Current())
	}
	return Source{}, fmt.Errorf("%s is not in any configured plugin catalog", slug)
}

// repairVersion picks the catalog artifact a repair installs: the installed
// version when the catalog still carries it, and otherwise — only when the host
// release itself changed — the version built for the current release.
//
// The catalog keeps one version per plugin, so after a release that bumped the
// plugins too, the installed version is gone. Insisting on it disabled every
// plugin on the first boot of the new release; the plugin moving with the host
// is exactly what an operator who installed that release asked for.
func repairVersion(versions []PluginVersion, installed, current string, releaseChanged bool) (PluginVersion, bool) {
	for _, v := range versions {
		if v.Version == installed {
			return v, true
		}
	}
	if !releaseChanged {
		return PluginVersion{}, false
	}
	for _, v := range versions {
		if v.BuiltFor == current {
			return v, true
		}
	}
	return PluginVersion{}, false
}

// keepOrDisable decides what a failed repair leaves behind. A refusal in
// preflight touched nothing and a rolled-back install restored the previous
// version, so both keep the plugin as it was; the rollback also records why.
// Anything else falls through to disableUnrepairable.
func (h *Handler) keepOrDisable(ctx context.Context, p InstalledPlugin, cause error, logger Logger) {
	switch {
	case errors.Is(cause, ErrUpgradeRefused):
		return
	case errors.Is(cause, ErrUpgradeRolledBack):
		if err := h.svc.SetRepairState(ctx, p.Slug, p.BuiltFor, cause.Error()); err != nil && logger != nil {
			logger.Error("plugins: could not record repair failure",
				"slug", p.Slug, "error", err.Error())
		}
	default:
		h.disableUnrepairable(ctx, p, cause, logger)
	}
}

// disableUnrepairable records why a plugin could not be repaired and switches it
// off, so the runtime does not dlopen a .so this host cannot load and the store
// can explain the state instead of showing an install that silently does nothing.
func (h *Handler) disableUnrepairable(ctx context.Context, p InstalledPlugin, cause error, logger Logger) {
	// MarkEnabled on a slug that is genuinely absent is not an error worth
	// logging, because the plugin is not there to disable.
	if err := h.svc.MarkEnabled(ctx, p.Slug, false); err != nil {
		if logger != nil && !errors.Is(err, ErrPluginNotFound) {
			logger.Warn("plugins: could not disable unrepairable plugin",
				"slug", p.Slug, "error", err.Error())
		}
	}
	reason := cause.Error()
	if errors.Is(cause, ErrBuiltForMismatch) {
		reason = fmt.Sprintf("catalog artifact is built for another release (host is %s)",
			appversion.Current())
	}
	if err := h.svc.SetRepairState(ctx, p.Slug, p.BuiltFor, reason); err != nil && logger != nil {
		logger.Error("plugins: could not record repair failure",
			"slug", p.Slug, "error", err.Error())
	}
}
