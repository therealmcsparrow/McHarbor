// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Shared helpers for plan-level UI logic (mandatory local storage,
// plan → input form mapping, default input shape). Kept in a
// dedicated file so both the per-container BackupsTab and the
// cross-environment Backups overview page can reuse them.

import type {
  ContainerBackupInput,
  ContainerBackupPlan,
} from "../hooks/useContainerBackups";

// mandatoryLocalStorage picks the storage location that must always
// be present on a plan's destination list — the default local backup
// location, falling back to the first local location with the
// canonical /mnt/backup base path, then any other local location.
// Returns null if there are no local locations configured at all.
export function mandatoryLocalStorage(
  locations: { id: string; locationType: string; basePath?: string }[],
) {
  return (
    locations.find((location) => location.id === "default-local-backup") ??
    locations.find(
      (location) =>
        location.locationType === "local" &&
        location.basePath === "/mnt/backup",
    ) ??
    locations.find((location) => location.locationType === "local") ??
    null
  );
}

export function planToInput(plan: ContainerBackupPlan): ContainerBackupInput {
  return {
    name: plan.name,
    storageLocationId: plan.storageLocationId ?? plan.storageLocationIds[0] ?? "",
    storageLocationIds: plan.storageLocationIds ?? [],
    includeConfig: plan.includeConfig,
    includeLogs: plan.includeLogs,
    includeFilesystem: plan.includeFilesystem,
    includeImage: plan.includeImage,
    selectedMounts: plan.selectedMounts ?? [],
    logTailLines: plan.logTailLines,
    cron: plan.cron ?? "",
    enabled: plan.enabled,
    retentionCount: plan.retentionCount,
    retentionDays: plan.retentionDays,
  };
}

export const EMPTY_BACKUP_INPUT: ContainerBackupInput = {
  name: "",
  storageLocationId: "",
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: "",
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
};

// withMandatoryStorage guarantees the mandatory local destination is
// in the plan's destinations list and is also the primary
// storageLocationId. Used by the create / edit form patches.
export function withMandatoryStorage(
  input: ContainerBackupInput,
  mandatoryStorageLocationId: string | undefined,
): ContainerBackupInput {
  if (!mandatoryStorageLocationId) return input;
  const ids = new Set(input.storageLocationIds ?? []);
  ids.add(mandatoryStorageLocationId);
  const storageLocationIds = Array.from(ids);
  return {
    ...input,
    storageLocationId: input.storageLocationId || mandatoryStorageLocationId,
    storageLocationIds,
  };
}