// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// rollbackDirName holds the install directories an upgrade set aside. It sits
// beside plugins/ rather than inside it, because the loader loads every
// directory under plugins/ and must never see two versions of one slug.
const rollbackDirName = "plugin-rollback"

// Preflight resolves, verifies, extracts and validates src without touching the
// installed plugin, so an upgrade learns the host refuses an artifact before it
// uninstalls anything. The caller installs or discards the result.
func (d *DefaultInstaller) Preflight(ctx context.Context, src Source, progress ProgressFunc) (*Staged, error) {
	if progress == nil {
		progress = noProgress
	}
	return d.stage(ctx, src, progress)
}

// InstallStaged installs a Staged from Preflight: the bytes that were validated
// are the bytes placed. It consumes st whether it succeeds or not.
func (d *DefaultInstaller) InstallStaged(ctx context.Context, st *Staged, progress ProgressFunc) (*Installed, error) {
	if st == nil || st.manifest == nil {
		return nil, errors.New("install staged: nothing was staged")
	}
	return d.installStaged(ctx, st, progress, 0)
}

// DiscardStaged removes a staged artifact an upgrade decided not to install.
func (d *DefaultInstaller) DiscardStaged(st *Staged) {
	if st == nil || st.Dir == "" {
		return
	}
	d.removeAll(st.Dir)
}

// asidePath is where an upgrade keeps the previous install of slug@version.
// Deterministic, so a boot after a crash finds it without any other record.
func (d *DefaultInstaller) asidePath(slug, version string) string {
	return filepath.Join(d.dataDir, rollbackDirName, slug+"@"+version)
}

// setAside moves an install directory into the rollback area, replacing a stale
// copy of the same version an earlier interrupted upgrade may have left.
func (d *DefaultInstaller) setAside(target, slug, version string) error {
	aside := d.asidePath(slug, version)
	if err := os.MkdirAll(filepath.Dir(aside), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(aside); err != nil {
		return err
	}
	if err := os.Rename(target, aside); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// HasPrevious reports whether an upgrade set slug@version aside and it is still
// there to roll back to.
func (d *DefaultInstaller) HasPrevious(slug, version string) bool {
	info, err := os.Stat(d.asidePath(slug, version))
	return err == nil && info.IsDir()
}

// RestorePrevious reinstates the install an upgrade set aside: directory, row,
// recorded build and target release, manifest contributions and preset grants.
// Migrations are not re-run; the set-aside uninstall recorded them in the ledger.
func (d *DefaultInstaller) RestorePrevious(ctx context.Context, prev PreviousInstall) error {
	if !d.platformOK {
		return ErrPlatformUnsupported
	}
	aside := d.asidePath(prev.Slug, prev.Version)
	mb, err := os.ReadFile(filepath.Join(aside, "plugin.yaml"))
	if err != nil {
		return fmt.Errorf("reading the set-aside manifest: %w", err)
	}
	var m plugin.Manifest
	if err := yaml.Unmarshal(mb, &m); err != nil {
		return fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	pluginsDir := filepath.Join(d.dataDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}
	target := filepath.Join(pluginsDir, prev.Slug+"@"+prev.Version)

	existing, err := d.svc.LookupVersion(ctx, prev.Slug)
	if err != nil {
		return err
	}
	switch existing {
	case "":
	case prev.Version:
		// The row survived: either the upgrade stopped between setting the
		// files aside and deleting the row, or a same-version install already
		// finished. Only the first needs the files back.
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		if err := os.Rename(aside, target); err != nil {
			return fmt.Errorf("reinstating files: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s@%s", ErrAlreadyInstalled, prev.Slug, existing)
	}
	// A failed install of the same version cleans its own target up; anything
	// still there is a half-placed tree that must not shadow the good one.
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("clearing %s: %w", target, err)
	}
	if err := os.Rename(aside, target); err != nil {
		return fmt.Errorf("reinstating files: %w", err)
	}
	// Every failure below moves the files back, so the next attempt still has
	// something to restore from.
	undo := func(cause error) error {
		if err := d.svc.DeleteRow(ctx, prev.Slug); err != nil && !errors.Is(err, ErrPluginNotFound) {
			d.warn("plugin rollback: removing the partial row", prev.Slug, err)
		}
		if err := d.setAside(target, prev.Slug, prev.Version); err != nil {
			d.warn("plugin rollback: setting the files aside again", prev.Slug, err)
		}
		return cause
	}
	if _, err := d.svc.InsertInstalledRowWithBuild(ctx, prev.Slug, prev.Version, prev.Build,
		m.Name, m.Author, m.Desc, prev.Signed); err != nil {
		return undo(err)
	}
	if err := d.svc.SetRepairState(ctx, prev.Slug, prev.BuiltFor, ""); err != nil {
		return undo(err)
	}
	if err := d.recordManifestContributions(ctx, prev.Slug, &m); err != nil {
		return undo(err)
	}
	if err := d.svc.GrantKeysToRoles(ctx, presetGrants(m.AddsPerms)); err != nil {
		return undo(fmt.Errorf("granting preset permissions: %w", err))
	}
	d.emitAudit(ctx, prev.Slug, "plugin.rollback", map[string]any{"version": prev.Version})
	d.syncCatalogs(ctx, prev.Slug)
	return nil
}

// FinishUpgrade drops what an upgrade kept for a rollback once the new version
// is in. The stored package goes only when the version moved: a same-version
// repair has just stored the new bytes under that very key.
func (d *DefaultInstaller) FinishUpgrade(ctx context.Context, slug, prevVersion, newVersion string) {
	d.removeAll(d.asidePath(slug, prevVersion))
	if prevVersion != newVersion {
		d.forgetPackage(ctx, slug, prevVersion)
	}
}
