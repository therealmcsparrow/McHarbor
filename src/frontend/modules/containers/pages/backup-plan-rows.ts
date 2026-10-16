// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerBackupPlan } from '@resources/hooks/useContainerBackups';
import type { BackupProviderPlan } from '@resources/hooks/useBackupProviderRuns';
import { SOURCE_CONTAINER } from './backup-run-rows';

export type BackupPlanRow = ContainerBackupPlan & {
  sourceKind: string;
  sourceSlug?: string;
  sourceLabel?: string;
  subjectName: string;
};

export function containerPlanRow(plan: ContainerBackupPlan): BackupPlanRow {
  return {
    ...plan,
    sourceKind: SOURCE_CONTAINER,
    subjectName: plan.containerName || plan.containerId.slice(0, 12),
  };
}

export function isContainerPlan(row: BackupPlanRow) {
  return row.sourceKind === SOURCE_CONTAINER;
}

export function providerPlanRow(plan: BackupProviderPlan): BackupPlanRow {
  const subjectId = plan.subjectId ?? '';
  return {
    id: plan.id,
    name: plan.name,
    environmentId: plan.environmentId ?? '',
    containerId: subjectId,
    containerName: plan.subjectName ?? '',
    storageLocationIds: [],
    includeConfig: false,
    includeLogs: false,
    includeFilesystem: false,
    includeImage: false,
    selectedMounts: [],
    cron: plan.cron,
    enabled: plan.enabled,
    retentionCount: plan.retentionCount,
    retentionDays: plan.retentionDays,
    lastRunAt: plan.lastRunAt,
    nextRunAt: plan.nextRunAt,
    sourceKind: plan.sourceKind,
    sourceSlug: plan.sourceSlug,
    sourceLabel: plan.sourceLabel,
    subjectName: plan.subjectName || subjectId,
    createdAt: '',
    updatedAt: '',
  };
}

export function mergeBackupPlanRows(
  containerPlans: ContainerBackupPlan[],
  providerPlans: BackupProviderPlan[],
): BackupPlanRow[] {
  const rows = [...containerPlans.map(containerPlanRow), ...providerPlans.map(providerPlanRow)];
  rows.sort((a, b) => a.name.localeCompare(b.name));
  return rows;
}
