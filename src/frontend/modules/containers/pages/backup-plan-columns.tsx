// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Column definitions for the backup *plans* table. Split out of
// backup-columns.tsx; cell rendering is unchanged, including the
// per-button `data-testid` hooks.

import { useMemo } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconClock, IconExternalLink, IconPencil, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { PERM } from '@core/auth/permissionKeys';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { describeCron } from '@resources/utils/schedule';
import type { ContainerBackupPlan } from '@resources/hooks/useContainerBackups';
import { isContainerPlan, type BackupPlanRow } from './backup-plan-rows';
import { PluginPlanActionsCell } from './plugin-backup-actions';
import { RunSourceCell } from './backup-source-cell';
import { useSourceLabel } from '../hooks/useSourceLabel';
import { DASH } from './backup-run-cells';
import { environmentName, formatTimestamp, retentionText } from './backups-page-helpers';
import { StaleIdBadge } from '../components/StaleIdBadge';
import { useBackupPlanOptionalColumns } from './useBackupPlanOptionalColumns';

type PlanColumnArgs = {
  envNameById: Map<string, string>;
  onEdit: (plan: ContainerBackupPlan) => void;
  onDelete: (plan: ContainerBackupPlan) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
};

export function useBackupPlanColumns({
  envNameById,
  onEdit,
  onDelete,
  onOpenContainer,
}: PlanColumnArgs): ColumnDef<BackupPlanRow, unknown>[] {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const formatPrefs = useLocaleFormat();
  const sourceLabel = useSourceLabel();
  const optionalColumns = useBackupPlanOptionalColumns();

  return useMemo(
    () => [
      {
        id: 'name',
        accessorFn: (plan) => plan.name,
        header: t('backups.plans.table.name'),
        cell: ({ row }) => <span className="font-medium text-foreground">{row.original.name}</span>,
      },
      {
        id: 'environment',
        accessorFn: (plan) =>
          environmentName(plan.environmentId, envNameById.get(plan.environmentId) ?? plan.environmentId.slice(0, 8)),
        header: t('backups.plans.table.environment'),
      },
      {
        id: 'source',
        accessorFn: (plan) => sourceLabel(plan),
        header: t('backups.runs.table.source'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <RunSourceCell row={row.original} label={sourceLabel(row.original)} />,
      },
      {
        id: 'container',
        accessorFn: (plan) => plan.subjectName,
        header: t('backups.runs.table.subject'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const plan = row.original;
          if (!isContainerPlan(plan)) {
            return <span className="font-medium text-foreground">{plan.subjectName}</span>;
          }
          return (
            <Link
              to={`/containers/${plan.containerId}`}
              onClick={(clickEvent) => {
                clickEvent.preventDefault();
                clickEvent.stopPropagation();
                onOpenContainer(plan.environmentId, plan.containerId);
              }}
              className="flex flex-col items-start gap-1 text-foreground hover:text-primary"
            >
              <span className="flex items-center gap-1">
                {plan.containerName || plan.containerId.slice(0, 12)}
                <IconExternalLink className="size-3.5 opacity-60" aria-hidden="true" />
              </span>
              {plan.containerIdStale && <StaleIdBadge />}
            </Link>
          );
        },
      },
      {
        id: 'schedule',
        accessorFn: (plan) => plan.cron ?? '',
        header: t('backups.plans.table.schedule'),
        cell: ({ row }) =>
          row.original.cron ? (
            <span className="flex items-center gap-1">
              <IconClock className="size-3.5 opacity-70" aria-hidden="true" />
              {describeCron(row.original.cron, tc)}
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">{DASH}</span>
          ),
      },
      {
        id: 'destinations',
        accessorFn: (plan) =>
          plan.storageLocationIds.length > 0
            ? t('backups.destinationsCount', { count: plan.storageLocationIds.length })
            : DASH,
        header: t('backups.plans.table.destinations'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
      },
      {
        id: 'retention',
        accessorFn: (plan) => retentionText(plan, t),
        header: t('backups.plans.table.retention'),
      },
      {
        id: 'nextRun',
        accessorFn: (plan) => formatTimestamp(plan.nextRunAt, formatPrefs),
        header: t('backups.plans.table.nextRun'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
      },
      {
        id: 'enabled',
        accessorFn: (plan) => (plan.enabled ? t('backups.enabled') : t('backups.disabled')),
        header: t('backups.plans.table.enabled'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={row.original.enabled ? 'default' : 'secondary'}>
            {row.original.enabled ? t('backups.enabled') : t('backups.disabled')}
          </Badge>
        ),
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: tc('labels.actions'),
        enableSorting: false,
        meta: { width: 'md', locked: true, noExport: true, label: tc('labels.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const plan = row.original;
          if (!isContainerPlan(plan)) return <PluginPlanActionsCell plan={plan} />;
          return (
            <div className="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('backups.plans.table.edit')}
                permission={PERM.BACKUPS_MANAGE}
                onClick={() => onEdit(plan)}
                data-testid={`plan-edit-${plan.id}`}
              >
                <IconPencil className="size-4" aria-hidden="true" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('backups.plans.table.delete')}
                permission={PERM.BACKUPS_DELETE}
                onClick={() => onDelete(plan)}
                data-testid={`plan-delete-${plan.id}`}
              >
                <IconTrash className="size-4 text-destructive" aria-hidden="true" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('backups.plans.table.open')}
                onClick={() => onOpenContainer(plan.environmentId, plan.containerId)}
              >
                <IconExternalLink className="size-4" aria-hidden="true" />
              </Button>
            </div>
          );
        },
      },
    ],
    [envNameById, formatPrefs, onDelete, onEdit, onOpenContainer, sourceLabel, t, tc, optionalColumns],
  );
}
