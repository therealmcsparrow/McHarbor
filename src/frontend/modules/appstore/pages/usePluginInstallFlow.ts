// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useStreamPluginAction } from '../hooks/useStreamPlugin';
import { isUnsignedVersion } from '../components/PluginSignature';
import type { PluginEntry, PluginVersion } from '../types/plugins';

const STREAM_OPTIONS = {};

export const UNSIGNED_REFUSED_CODE = 'err.plugin.unsigned_confirmation_required';

interface InstallTarget {
  name: string;
  url: string;
  checksum?: string;
  // The catalog entry's signature for the chosen release. The host
  // verifies against it before extracting; without it the install is
  // recorded unsigned even when the archive is signed.
  signature?: string;
  allowUnsigned?: boolean;
}

// PendingInstall is the picker-phase handoff: when the user clicks
// Install on a multi-version card we open the modal with this object
// + the user's chosen `selectedVersion`. They confirm, we promote it
// into InstallTarget + start SSE, the dialog auto-transitions.
export interface PendingInstall {
  entry: PluginEntry;
  versions: PluginVersion[];
}

// Owns the SSE-driven install / update flow behind PluginInstallDialog on
// the plugin store page: picker phase, install target, update target and
// the stream state that drives the progress UI.
export function usePluginInstallFlow(onDispatch: () => void) {
  const [installTarget, setInstallTarget] = useState<InstallTarget | null>(null);
  // Modal picker phase — set when the user clicks Install on a
  // multi-version card. Cleared by confirmInstall (→ SSE starts) or
  // closeInstallDialog (user backed out).
  const [pendingInstall, setPendingInstall] = useState<PendingInstall | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<PluginVersion | null>(null);
  const [updateTargetName, setUpdateTargetName] = useState<string | null>(null);
  const [updateSlug, setUpdateSlug] = useState<string | null>(null);

  // SSE-driven install flow. Opens PluginInstallDialog; the
  // mutation-only useInstallPlugin bypassed the modal and never
  // reported progress — replaced here so the user sees the
  // download/verify/extract/migrate/persist stages.
  const stream = useStreamPluginAction(STREAM_OPTIONS);
  const startInstall = stream.startInstall;

  // Plugins and Docker apps share the same /api/appstore catalog feed
  // but not the same install path. Docker apps go through the
  // multi-step InstallDialog (port mapping, volume mounts, env vars).
  // Plugins ship a manifest + config schema and go through the
  // dedicated plugin install SSE endpoint at
  // /api/appstore/plugins/install/stream — the dialog renders
  // progress + abort while the backend runs verify → extract →
  // migrate → persist.
  //
  // The PluginCard no longer picks a version inline; the install modal
  // owns the version selector. When the user clicks Install we open
  // the modal in picker mode (versions > 1) or jump straight to SSE
  // start (single-version). The picker UI threads the chosen URL +
  // checksum back into `installTarget`, then the existing useEffect
  // fires startInstall and the dialog auto-transitions to progress.
  const dispatchInstall = (entry: PluginEntry) => {
    onDispatch();
    setUpdateTargetName(null);
    // Synthesise a fallback versions[] when the catalog entry uses the
    // legacy single-version shape. The catalog loader guarantees
    // versions[0] is the latest either way.
    const versions: PluginVersion[] =
      entry.versions.length > 0
        ? entry.versions
        : [
            {
              version: entry.version,
              download_url: entry.download_url,
              checksum: entry.checksum,
              signature: entry.signature,
            },
          ];

    // Single-version and signed: skip the picker, start SSE right away.
    // An unsigned release still opens the dialog, because the warning and
    // the acknowledgement live there — installing it silently is the one
    // outcome the "at your own risk" policy must not produce.
    const only = versions.length === 1 ? versions[0] : undefined;
    if (only && !isUnsignedVersion(only)) {
      setInstallTarget({
        name: `${entry.name} v${only.version}`,
        url: only.download_url,
        checksum: only.checksum,
        signature: only.signature,
      });
      return;
    }

    setPendingInstall({ entry, versions });
    // Pin the default to versions[0]. Fall back to `entry.version`
    // metadata when versions[0] is somehow undefined (defensive — the
    // length check above guarantees at least one element, but
    // noUncheckedIndexedAccess still types it as PluginVersion |
    // undefined). The Install button's `disabled` check and
    // confirmInstall() both rely on this being a real PluginVersion.
    setSelectedVersion(versions[0] ?? null);
  };

  // User clicked Install in the picker modal. Promote the chosen
  // version into installTarget so the existing useEffect fires
  // stream.startInstall(url, checksum) and the dialog transitions
  // into progress mode.
  //
  // Defensive fallback: if selectedVersion is somehow null (Select
  // never fired onChange, or a re-render raced the state batch), use
  // the catalog's latest (versions[0]). Without this guard the picker
  // button does nothing on click, which was the previous bug.
  const confirmInstall = () => {
    if (!pendingInstall) return;
    const chosen = selectedVersion ?? pendingInstall.versions[0] ?? null;
    if (!chosen) return;
    setInstallTarget({
      name: `${pendingInstall.entry.name} v${chosen.version}`,
      url: chosen.download_url,
      checksum: chosen.checksum,
      signature: chosen.signature,
      allowUnsigned: isUnsignedVersion(chosen),
    });
    setPendingInstall(null);
    setSelectedVersion(null);
  };

  // Update path: hits /api/appstore/plugins/{slug}/update/stream.
  // The host tears down the existing installation and reinstalls
  // at the chosen version, so the user gets a single-click update
  // without having to uninstall first. Wired from
  // useStreamPluginAction().startUpdate so the same SSE dialog
  // that shows install progress also shows update progress.
  //
  // Future: a `version` query param so the operator can pin the
  // target version. Today the host always picks the latest catalog
  // entry by slug — there is no version picker on the update path.
  const dispatchUpdate = (entry: PluginEntry) => {
    stream.reset();
    onDispatch();
    setUpdateTargetName(entry.name);
    setUpdateSlug(entry.slug);
    void stream.startUpdate(entry.slug);
  };

  // Trigger SSE install once the target is set. PluginsPage uses the
  // same pattern; duplicated here because the install trigger needs to
  // live alongside the dialog that displays its progress, and the
  // store page owns both.
  useEffect(() => {
    if (!installTarget) return;
    void startInstall({
      url: installTarget.url,
      checksum: installTarget.checksum,
      signature: installTarget.signature,
      allow_unsigned: installTarget.allowUnsigned,
    });
  }, [installTarget, startInstall]);

  const unsignedRefused = stream.state.errorCode === UNSIGNED_REFUSED_CODE;

  const confirmUnsigned = () => {
    if (!unsignedRefused || stream.state.running) return;
    if (installTarget) {
      setInstallTarget({ ...installTarget, allowUnsigned: true });
      return;
    }
    if (updateSlug) {
      void stream.startUpdate(updateSlug, { allowUnsigned: true });
    }
  };

  const closeInstallDialog = () => {
    stream.reset();
    setInstallTarget(null);
    // Also drop picker-phase state so re-opening the modal for a
    // different plugin starts fresh instead of inheriting the
    // previous plugin's pending versions.
    setPendingInstall(null);
    setSelectedVersion(null);
    setUpdateTargetName(null);
    setUpdateSlug(null);
  };

  // Show the dialog for picker phase (pendingInstall), install
  // running (installTarget), and update running (stream.state).
  // Update path doesn't set installTarget because the request
  // body is the slug, not a URL — the stream alone owns open
  // in that case.
  const dialogOpen = !!pendingInstall || !!installTarget || stream.state.running || !!stream.state.progress;

  return {
    stream,
    installTarget,
    pendingInstall,
    selectedVersion,
    setSelectedVersion,
    updateTargetName,
    dialogOpen,
    dispatchInstall,
    confirmInstall,
    dispatchUpdate,
    closeInstallDialog,
    unsignedRefused,
    confirmUnsigned,
  };
}

/**
 * The install dialog's title, which differs across three phases: picking
 * a version, installing a fresh plugin, and updating an installed one.
 * Lives here rather than inline in the page so the page stays a layout.
 */
export function useInstallDialogTitle(flow: {
  pendingInstall: { entry: { name: string } } | null;
  installTarget: { name: string } | null;
  updateTargetName: string | null;
}): string {
  const { t } = useTranslation('plugins');
  if (flow.pendingInstall) {
    return t('install.title', { name: flow.pendingInstall.entry.name });
  }
  if (flow.installTarget) {
    return t('install.title', { name: flow.installTarget.name });
  }
  return t('install.update_title', { name: flow.updateTargetName ?? '' });
}
