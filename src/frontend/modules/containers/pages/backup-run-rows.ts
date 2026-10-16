// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import type { BackupProviderRun } from '@resources/hooks/useBackupProviderRuns';

export const SOURCE_CONTAINER = 'container';

export type BackupRunRow = ContainerBackupRun & {
  sourceKind: string;
  sourceSlug?: string;
  sourceLabel?: string;
  subjectName: string;
  providerKind?: string;
  warning?: string;
};

export function containerRunRow(run: ContainerBackupRun): BackupRunRow {
  return {
    ...run,
    sourceKind: SOURCE_CONTAINER,
    subjectName: run.containerName || run.containerId.slice(0, 12),
  };
}

export function isContainerRun(row: BackupRunRow) {
  return row.sourceKind === SOURCE_CONTAINER;
}

export function providerRunRow(run: BackupProviderRun): BackupRunRow {
  const subjectId = run.subjectId ?? '';
  return {
    id: run.id,
    operation: run.operation === 'restore' ? 'restore' : 'backup',
    environmentId: run.environmentId ?? '',
    containerId: subjectId,
    containerName: run.subjectName,
    status: normaliseStatus(run.status),
    archiveSize: run.sizeBytes,
    error: run.error,
    destinations: (run.destinations ?? []).map((destinationName, index) => ({
      id: `${run.id}-${index}`,
      runId: run.id,
      storageLocationName: destinationName,
      locationType: 'local',
      status: 'success' as const,
      path: '',
      createdAt: run.startedAt,
      updatedAt: run.startedAt,
    })),
    startedAt: run.startedAt,
    completedAt: run.completedAt,
    durationMs: run.durationMs,
    createdAt: run.startedAt,
    updatedAt: run.completedAt ?? run.startedAt,
    sourceKind: run.sourceKind,
    sourceSlug: run.sourceSlug,
    sourceLabel: run.sourceLabel,
    subjectName: run.subjectName || subjectId,
    providerKind: run.kind,
    warning: run.warning,
  };
}

export function mergeBackupRunRows(
  containerRuns: ContainerBackupRun[],
  providerRuns: BackupProviderRun[],
): BackupRunRow[] {
  const rows = [...containerRuns.map(containerRunRow), ...providerRuns.map(providerRunRow)];
  rows.sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime());
  return rows;
}

function normaliseStatus(rawStatus: string): ContainerBackupRun['status'] {
  switch (rawStatus) {
    case 'running':
    case 'success':
    case 'failure':
    case 'cancelled':
      return rawStatus;
    default:
      return 'failure';
  }
}
