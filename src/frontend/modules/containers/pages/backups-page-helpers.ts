// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerBackupPlan, ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { containerBackupDownloadUrl } from '@resources/hooks/useContainerBackups';
import { formatDate, type LocaleFormat } from '@resources/utils/format';
import { type ContainerBackupRunDestination } from '@resources/hooks/useContainerBackups';

// formatBytes renders a byte count as a human-readable string with
// binary unit suffixes. Zero and negative values render as "0 B".
export function formatBytes(value: number) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

// formatTimestamp renders an RFC3339 timestamp with the active
// locale and the user's chosen time + date format. Returns an
// em-dash for empty values; `formatDate` already falls back to the
// raw string when the Date constructor throws.
export function formatTimestamp(value?: string, prefs?: Partial<LocaleFormat>) {
  if (!value) return '—';
  const formatted = formatDate(value, prefs);
  return formatted === '-' ? value : formatted;
}

// destinationLabel prefers the human-readable storage location
// name on a backup run destination, then the id, then the
// caller-provided fallback. Used by the runs table when rendering
// the destination column.
export function destinationLabel(destination: ContainerBackupRunDestination, fallback: string) {
  if (destination.storageLocationName) return destination.storageLocationName;
  if (destination.storageLocationId) return destination.storageLocationId;
  return fallback;
}

// statusBadgeVariant maps a backup run status onto the
// shared <Badge> variant so the badge color matches the rest of
// the app.
export function statusBadgeVariant(runStatus: string): 'default' | 'destructive' | 'secondary' | 'outline' {
  switch (runStatus) {
    case 'running':
      return 'outline';
    case 'success':
      return 'default';
    case 'failure':
      return 'destructive';
    case 'cancelled':
      return 'secondary';
    default:
      return 'secondary';
  }
}

// environmentName is a placeholder hook for the future when the
// Backups overview might run with a per-row environment id that
// differs from the current global env. Today it always returns the
// fallback because the global env is the only context. Kept as
// a helper so call sites read naturally.
export function environmentName(envId: string | undefined, fallback: string) {
  if (!envId) return fallback;
  return fallback;
}

// ---------------------------------------------------------------------------
// Moved here from backups-page-tables.tsx: both that file and backup-columns.tsx
// need them, and leaving them in the table file made the imports circular.
// ---------------------------------------------------------------------------

export function retentionText(plan: ContainerBackupPlan, t: (key: string, options?: Record<string, number>) => string) {
  if (plan.retentionCount > 0 && plan.retentionDays > 0) {
    return t('backups.retentionCountDays', {
      count: plan.retentionCount,
      days: plan.retentionDays,
    });
  }
  if (plan.retentionCount > 0) {
    return t('backups.retentionCountOnly', { count: plan.retentionCount });
  }
  if (plan.retentionDays > 0) {
    return t('backups.retentionDaysOnly', { count: plan.retentionDays, days: plan.retentionDays });
  }
  return t('backups.retentionForever');
}

// StaleIdBadge surfaces the containerIdStale flag the server sets when the
// auto-refresh on read changed the container_id because the stored id no longer
// matched a live container. The persisted row has already been updated by the
// time the flag is set, so the UI just shows a small "Re-linked" badge to make
// the re-link visible to the operator.
// ---------------------------------------------------------------------------
// Run type ("Full Backup" / "Partial Restore") and the item vocabulary behind
// it. The server records what a run captured and what was on offer; `scope` is
// its verdict, absent for runs written before that inventory was kept.
// ---------------------------------------------------------------------------

// runHasError reports whether a run has anything the error dialog could show.
// A run can finish "success" at the archive step and still fail to reach one of
// its destinations, so the error affordance follows the errors, not the status
// badge. Shared so the Backups table and the container Backups tab cannot
// disagree about when the button appears.
export function runHasError(run: { status: string; error?: string; destinations?: { status: string }[] }) {
  return (
    run.status === 'failure' ||
    !!run.error?.trim() ||
    (run.destinations ?? []).some((destination) => destination.status === 'failure')
  );
}

// runTypeKey maps a run onto a `backups.runType.*` translation key. Runs with
// no recorded scope fall back to the bare operation, so the column reads
// "Backup" / "Restore" rather than claiming a completeness it cannot know.
export function runTypeKey(run: { operation: 'backup' | 'restore'; scope?: 'full' | 'partial' }) {
  if (run.scope === 'full') {
    return run.operation === 'restore' ? 'backups.runType.fullRestore' : 'backups.runType.fullBackup';
  }
  if (run.scope === 'partial') {
    return run.operation === 'restore' ? 'backups.runType.partialRestore' : 'backups.runType.partialBackup';
  }
  return run.operation === 'restore' ? 'backups.runType.restore' : 'backups.runType.backup';
}

// backupItemLabel turns one item key into display text. Mount keys carry the
// container path after the prefix, which is data rather than copy — it is
// interpolated into the translated label, not translated itself.
export function backupItemLabel(item: string, t: (key: string, options?: Record<string, string>) => string) {
  if (item.startsWith('mount:')) {
    return t('backups.items.mount', { path: item.slice('mount:'.length) });
  }
  // Per-file and per-folder selections are keyed `file:<mount>:<inner path>`
  // (e.g. `file:/var/www/html:html/console.php`). The inner path is relative
  // to the archived tar's own root, which is not the mount path, so the two
  // are shown side by side rather than joined into a container path that
  // would not exist.
  if (item.startsWith('file:') || item.startsWith('folder:')) {
    const isFolder = item.startsWith('folder:');
    const rest = item.slice(isFolder ? 'folder:'.length : 'file:'.length);
    const separator = rest.indexOf(':');
    if (separator > 0) {
      return t(isFolder ? 'backups.items.folder' : 'backups.items.file', {
        path: rest.slice(separator + 1),
        mount: rest.slice(0, separator),
      });
    }
    return rest;
  }
  switch (item) {
    case 'config':
      return t('backups.items.config');
    case 'image':
      return t('backups.items.image');
    case 'filesystem':
      return t('backups.items.filesystem');
    case 'logs':
      return t('backups.items.logs');
    default:
      return item;
  }
}

export function triggerBrowserDownload(run: ContainerBackupRun) {
  const anchor = document.createElement('a');
  anchor.href = containerBackupDownloadUrl(run.id);
  anchor.download = `${run.containerName || run.containerId.slice(0, 12)}-${run.id}.mcharbor.tar`;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
}

export function bulkDownloadRuns(runs: ContainerBackupRun[]) {
  for (const run of runs) {
    triggerBrowserDownload(run);
  }
}
