// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ErrUpgradeRefused marks an upgrade the new artifact failed in preflight. The
// installed plugin was not touched; the cause is wrapped alongside it.
var ErrUpgradeRefused = errors.New("plugin upgrade refused before anything was uninstalled")

// ErrUpgradeRolledBack marks an upgrade whose install failed after the
// uninstall, and whose previous version, enabled flag and grants were reinstated.
var ErrUpgradeRolledBack = errors.New("plugin upgrade failed and the previous version was restored")

// upgrade replaces the installed slug with src, the one path both the Store
// Update and the boot repair take. Order is the whole point: validate the new
// artifact, persist what a rollback needs, set the old install aside, install,
// and only then restore grants and drop the old copy. A refusal before the
// uninstall touches nothing; a failure after it reinstates the previous version.
func (h *Handler) upgrade(ctx context.Context, slug string, src Source, progress ProgressFunc) (*Installed, error) {
	if progress == nil {
		progress = noProgress
	}
	up, ok := h.installer.(Upgrader)
	if !ok {
		return nil, fmt.Errorf("%w: %w: this installer cannot upgrade safely", ErrUpgradeRefused, ErrPlatformUnsupported)
	}

	progress("preflight", 2, "validating the new artifact")
	st, err := up.Preflight(ctx, src, progress)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpgradeRefused, err)
	}
	if st.Slug != slug {
		up.DiscardStaged(st)
		return nil, fmt.Errorf("%w: %w: artifact is %q, not %q", ErrUpgradeRefused, ErrManifestInvalid, st.Slug, slug)
	}

	prev, enabled, installed, err := h.svc.installedState(ctx, slug)
	if err != nil {
		up.DiscardStaged(st)
		return nil, fmt.Errorf("%w: %w", ErrUpgradeRefused, err)
	}
	if !installed {
		inst, err := up.InstallStaged(ctx, st, progress)
		if err != nil {
			return nil, err
		}
		h.applyPendingSnapshot(ctx, slug)
		return inst, nil
	}

	snap, err := h.snapshotFor(ctx, prev, enabled)
	if err == nil {
		err = h.svc.SaveUpdateSnapshot(ctx, snap)
	}
	if err != nil {
		up.DiscardStaged(st)
		return nil, fmt.Errorf("%w: persisting the grant snapshot: %w", ErrUpgradeRefused, err)
	}

	progress("uninstall", 5, "uninstalling existing "+slug+"@"+prev.Version)
	if _, err := h.installer.Uninstall(ctx, slug,
		UninstallOptions{PurgeData: false, KeepTables: true, SetAside: true}, progress); err != nil {
		up.DiscardStaged(st)
		return nil, h.rollback(ctx, up, snap, fmt.Errorf("uninstalling: %w", err), progress)
	}

	inst, err := up.InstallStaged(ctx, st, progress)
	if err != nil {
		return nil, h.rollback(ctx, up, snap, fmt.Errorf("installing: %w", err), progress)
	}

	if err := h.restoreSnapshot(ctx, snap); err != nil {
		// The new version is in; the snapshot stays so the next boot retries.
		slog.Warn("plugin upgrade: restoring grants failed, retrying at next boot",
			"slug", slug, "error", err)
	}
	up.FinishUpgrade(ctx, slug, prev.Version, inst.Version)
	return inst, nil
}

// upgradeOutcome is the client-safe text for a failed upgrade. It names what
// happened to the installed plugin; the cause stays in the server log.
func upgradeOutcome(err error) string {
	switch {
	case errors.Is(err, ErrUpgradeRefused):
		return "update refused; the installed version was not changed"
	case errors.Is(err, ErrUpgradeRolledBack):
		return "update failed; the previous version was restored"
	default:
		return "update failed"
	}
}

// snapshotFor reads every role grant of the plugin's keys. A failed read refuses
// the upgrade: proceeding would strip grants nothing could put back.
func (h *Handler) snapshotFor(ctx context.Context, prev PreviousInstall, enabled bool) (UpdateSnapshot, error) {
	keys, err := h.svc.PermissionsForPlugin(ctx, prev.Slug)
	if err != nil {
		return UpdateSnapshot{}, err
	}
	grants, err := h.svc.RoleGrantsForKeys(ctx, keys)
	if err != nil {
		return UpdateSnapshot{}, err
	}
	return UpdateSnapshot{Previous: prev, Enabled: enabled, Grants: grants}, nil
}

// rollback reinstates the version snap records after cause cut an upgrade short,
// and returns the error the caller reports: ErrUpgradeRolledBack with the cause,
// or both errors when the rollback itself failed and the snapshot is kept.
func (h *Handler) rollback(ctx context.Context, up Upgrader, snap UpdateSnapshot, cause error, progress ProgressFunc) error {
	slug := snap.Previous.Slug
	progress("rollback", 90, "restoring "+slug+"@"+snap.Previous.Version)
	if err := h.reinstate(ctx, up, snap); err != nil {
		slog.Error("plugin upgrade: rollback failed; the snapshot is kept for the next boot",
			"slug", slug, "cause", cause, "error", err)
		return errors.Join(cause, fmt.Errorf("rolling back: %w", err))
	}
	h.notifyChange(slug)
	slog.Warn("plugin upgrade: rolled back to the previous version",
		"slug", slug, "version", snap.Previous.Version, "cause", cause)
	return fmt.Errorf("%w: %w", ErrUpgradeRolledBack, cause)
}

// reinstate brings back the previous install when it is gone (or only half
// gone) and then its grants and enabled flag.
func (h *Handler) reinstate(ctx context.Context, up Upgrader, snap UpdateSnapshot) error {
	prev := snap.Previous
	installed, err := h.svc.LookupVersion(ctx, prev.Slug)
	if err != nil {
		return err
	}
	if (installed == "" || installed == prev.Version) && up.HasPrevious(prev.Slug, prev.Version) {
		if err := up.RestorePrevious(ctx, prev); err != nil {
			return err
		}
	} else if installed == "" {
		return fmt.Errorf("%s@%s is no longer on disk to restore", prev.Slug, prev.Version)
	}
	return h.restoreSnapshot(ctx, snap)
}

// restoreSnapshot re-grants the snapshot's keys, restores the enabled flag the
// install reset, and clears the snapshot.
func (h *Handler) restoreSnapshot(ctx context.Context, snap UpdateSnapshot) error {
	slug := snap.Previous.Slug
	if err := h.svc.GrantKeysToRoles(ctx, snap.Grants); err != nil {
		return fmt.Errorf("restoring role grants: %w", err)
	}
	if err := h.svc.MarkEnabled(ctx, slug, snap.Enabled); err != nil {
		return fmt.Errorf("restoring enabled state: %w", err)
	}
	return h.svc.ClearUpdateSnapshot(ctx, slug)
}

// applyPendingSnapshot restores the grants an interrupted upgrade left behind
// once the plugin is installed again by any path.
func (h *Handler) applyPendingSnapshot(ctx context.Context, slug string) {
	snap, err := h.svc.UpdateSnapshotFor(ctx, slug)
	if err != nil {
		slog.Warn("plugin install: reading a pending update snapshot failed", "slug", slug, "error", err)
		return
	}
	if snap == nil {
		return
	}
	if err := h.restoreSnapshot(ctx, *snap); err != nil {
		slog.Warn("plugin install: restoring a pending update snapshot failed", "slug", slug, "error", err)
		return
	}
	slog.Info("plugin install: restored the role grants an interrupted upgrade left", "slug", slug)
}

// RecoverInterruptedUpgrades finishes or undoes every upgrade a restart cut
// short, from its persisted snapshot. An installed plugin gets its grants and
// enabled flag back; a missing one is reinstated from its set-aside copy; one
// with no copy keeps its snapshot until it is installed again.
func (h *Handler) RecoverInterruptedUpgrades(ctx context.Context, logger Logger) {
	snaps, err := h.svc.PendingUpdateSnapshots(ctx)
	if err != nil {
		if logger != nil {
			logger.Error("plugins: reading interrupted upgrades failed", "error", err.Error())
		}
		return
	}
	up, ok := h.installer.(Upgrader)
	for _, snap := range snaps {
		prev := snap.Previous
		installed, err := h.svc.LookupVersion(ctx, prev.Slug)
		if err != nil {
			if logger != nil {
				logger.Error("plugins: interrupted upgrade not recovered", "slug", prev.Slug, "error", err.Error())
			}
			continue
		}
		if installed == "" && (!ok || !up.HasPrevious(prev.Slug, prev.Version)) {
			if logger != nil {
				logger.Warn("plugins: interrupted upgrade left no copy to restore; "+
					"its role grants return when the plugin is installed again",
					"slug", prev.Slug, "version", prev.Version)
			}
			continue
		}
		if ok {
			err = h.reinstate(ctx, up, snap)
		} else {
			err = h.restoreSnapshot(ctx, snap)
		}
		if err != nil {
			if logger != nil {
				logger.Error("plugins: interrupted upgrade not recovered", "slug", prev.Slug, "error", err.Error())
			}
			continue
		}
		if ok && installed != "" {
			up.FinishUpgrade(ctx, prev.Slug, prev.Version, installed)
		}
		h.notifyChange(prev.Slug)
		if logger != nil {
			logger.Warn("plugins: recovered an interrupted upgrade",
				"slug", prev.Slug, "previous", prev.Version, "installed", installed)
		}
	}
}
