// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { formatDate } from '@resources/utils/format';
import { ResourcePicker } from './ResourcePicker';

type BackupRunItem = {
  id: string;
  operation: string;
  status: string;
  containerName?: string;
  containerId: string;
  archiveSize: number;
  startedAt: string;
  completedAt?: string;
};

interface BackupRunSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
  containerId?: string;
  /** Restrict the list to completed backup runs (the only ones that can be restored or downloaded). */
  completedOnly?: boolean;
}

function formatSize(bytes: number): string {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

/** Picks a container backup run (by id) from the selected environment. */
export function BackupRunSelect({ value, onChange, envId, containerId, completedOnly }: BackupRunSelectProps) {
  const { t } = useTranslation('common');

  const { data: runs, isLoading } = useQuery({
    queryKey: ['backup-runs-select', envId ?? ''],
    queryFn: () =>
      api.get<BackupRunItem[]>('/container-backups/runs', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 15_000,
    enabled: !!envId,
  });

  const visibleRuns = (runs ?? []).filter((run) => {
    if (
      containerId &&
      run.containerName !== containerId &&
      run.containerId !== containerId &&
      !run.containerId.startsWith(containerId)
    ) {
      return false;
    }
    return completedOnly ? run.status === 'completed' && run.operation === 'backup' : true;
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      placeholder={t('workflows.selectBackupRun')}
      emptyLabel={t('workflows.noBackupRunsFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={visibleRuns.map((run) => ({
        value: run.id,
        title: `${run.containerName || run.containerId.slice(0, 12)} - ${formatDate(run.startedAt)}`,
        subtitle: `${run.operation} - ${formatSize(run.archiveSize)} - ${run.id.slice(0, 12)}`,
        badge: run.status,
        tone: run.status === 'completed' ? 'success' : run.status === 'failed' ? 'danger' : 'default',
      }))}
    />
  );
}
