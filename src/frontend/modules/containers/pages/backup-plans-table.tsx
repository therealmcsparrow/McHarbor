// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCalendarEvent, IconTrash } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import type { BatchAction } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { type ContainerBackupPlan } from '@resources/hooks/useContainerBackups';
import { isContainerPlan, type BackupPlanRow } from './backup-plan-rows';
import { useSourceLabel } from '../hooks/useSourceLabel';
import { useBackupPlanColumns } from './backup-plan-columns';
import { environmentName } from './backups-page-helpers';

export function BackupPlansTable({
  plans,
  loading,
  envNameById,
  onEdit,
  onDelete,
  onOpenContainer,
  onBulkDelete,
}: {
  plans: BackupPlanRow[];
  loading: boolean;
  envNameById: Map<string, string>;
  onEdit: (plan: ContainerBackupPlan) => void;
  onDelete: (plan: ContainerBackupPlan) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
  onBulkDelete: (plans: ContainerBackupPlan[]) => void;
}) {
  const { t } = useTranslation('containers');
  const planSourceLabel = useSourceLabel();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const columns = useBackupPlanColumns({ envNameById, onEdit, onDelete, onOpenContainer });

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: t('backups.bulkDelete'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => onBulkDelete((rows as BackupPlanRow[]).filter(isContainerPlan)),
      },
    ],
    [onBulkDelete, t],
  );

  return (
    <TablePage
      id="backup-plans"
      header={false}
      title={t('backups.tabPlans')}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('backups.plans.table.name'),
        fields: (plan) => [
          plan.name,
          plan.subjectName,
          plan.containerId,
          planSourceLabel(plan),
          environmentName(plan.environmentId, envNameById.get(plan.environmentId) ?? ''),
        ],
      }}
      toolbar={{
        columns: true,
        density: true,
        export: { filename: 'mcharbor-backup-plans' },
      }}
      columns={columns}
      data={plans}
      loading={loading && plans.length === 0}
      empty={{
        icon: <IconCalendarEvent className="size-8" />,
        title: t('backups.plans.emptyTitle'),
        description: t('backups.plans.emptyHint'),
      }}
      selectable
      batchActions={batchActions}
      getRowId={(plan) => plan.id}
      getRowTestId={(plan) => `plan-row-${plan.id}`}
    />
  );
}
