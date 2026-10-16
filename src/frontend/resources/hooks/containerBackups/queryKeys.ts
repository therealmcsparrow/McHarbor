// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const containerBackupKeys = {
  runsAll: ['container-backup-runs'] as const,
  runs: (envId: string | null | undefined, containerId: string | undefined) =>
    ['container-backup-runs', envId, containerId] as const,
  plansAll: ['container-backup-plans'] as const,
};
