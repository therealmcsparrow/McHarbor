// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Shared helpers for plan-level UI logic (mandatory local storage,
// plan → input form mapping, default input shape). Kept in a
// dedicated file so both the per-container BackupsTab and the
// cross-environment Backups overview page can reuse them.

import type { ContainerBackupInput, ContainerBackupPlan } from '@resources/hooks/useContainerBackups';

// defaultLocalStorage picks the storage location a new plan starts
// with selected — the default local backup location, falling back to
// the first local location with the canonical /mnt/backup base path,
// then any other local location. Returns null if there are no local
// locations configured at all.
//
// It is a *default*, not a mandate: the user may switch it off, and
// the backend no longer re-adds it. What is enforced instead is that
// at least one destination remains — see hasDestination.
export function defaultLocalStorage(locations: { id: string; locationType: string; basePath?: string }[]) {
  return (
    locations.find((loc2) => loc2.id === 'default-local-backup') ??
    locations.find(
      (locationValue) => locationValue.locationType === 'local' && locationValue.basePath === '/mnt/backup',
    ) ??
    locations.find((loc) => loc.locationType === 'local') ??
    null
  );
}

export function planToInput(plan: ContainerBackupPlan): ContainerBackupInput {
  return {
    name: plan.name,
    storageLocationId: plan.storageLocationId ?? plan.storageLocationIds[0] ?? '',
    storageLocationIds: plan.storageLocationIds ?? [],
    includeConfig: plan.includeConfig,
    includeLogs: plan.includeLogs,
    includeFilesystem: plan.includeFilesystem,
    includeImage: plan.includeImage,
    selectedMounts: plan.selectedMounts ?? [],
    logTailLines: plan.logTailLines,
    cron: plan.cron ?? '',
    enabled: plan.enabled,
    retentionCount: plan.retentionCount,
    retentionDays: plan.retentionDays,
  };
}

export const EMPTY_BACKUP_INPUT: ContainerBackupInput = {
  name: '',
  storageLocationId: '',
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: '',
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
};

// withPrimaryDestination keeps the legacy single `storageLocationId`
// in step with the destination list: it is always the first entry, and
// empty when the list is. It adds nothing of its own — clearing the
// last destination really does leave the input empty, which is what
// hasDestination then reports to the button gate.
export function withPrimaryDestination(input: ContainerBackupInput): ContainerBackupInput {
  const storageLocationIds = input.storageLocationIds ?? [];
  return {
    ...input,
    storageLocationId: storageLocationIds[0] ?? '',
    storageLocationIds,
  };
}

// hasDestination is the single source of truth for "can this backup run
// or be scheduled at all". The backend rejects an empty list with
// err.container.backup_destination_required; this keeps the user from
// getting there.
export function hasDestination(input: ContainerBackupInput): boolean {
  return (input.storageLocationIds ?? []).length > 0;
}
