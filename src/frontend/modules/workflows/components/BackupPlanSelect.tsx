// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { ResourcePicker } from './ResourcePicker';

type BackupPlanItem = {
  id: string;
  name: string;
  containerName: string;
  containerId: string;
  enabled: boolean;
  cron?: string;
  lastRunAt?: string;
};

interface BackupPlanSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
  containerId?: string;
}

function matchesContainer(containerName: string, id: string, filter?: string): boolean {
  if (!filter) return true;
  return containerName === filter || id === filter || id.startsWith(filter);
}

/** Picks a saved container backup plan (by id) from the selected environment. */
export function BackupPlanSelect({ value, onChange, envId, containerId }: BackupPlanSelectProps) {
  const { t } = useTranslation('common');

  const { data: plans, isLoading } = useQuery({
    queryKey: ['backup-plans-select', envId ?? ''],
    queryFn: () =>
      api.get<BackupPlanItem[]>('/container-backups', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      placeholder={t('workflows.selectBackupPlan')}
      emptyLabel={t('workflows.noBackupPlansFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={(plans ?? [])
        .filter((plan) => matchesContainer(plan.containerName, plan.containerId, containerId))
        .map((plan) => ({
          value: plan.id,
          title: plan.name,
          subtitle: `${plan.containerName || plan.containerId}${plan.cron ? ` - ${plan.cron}` : ''}`,
          badge: plan.enabled ? t('workflows.enabledBadge') : t('workflows.disabledBadge'),
          tone: plan.enabled ? 'success' : 'default',
        }))}
    />
  );
}
