// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconPlayerPlay, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { describeCron } from '@resources/utils/schedule';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { ExportSchedule } from '../hooks/useExportSchedules';

export type ScheduleRowActions = {
  running: boolean;
  onRun: (s: ExportSchedule) => void;
  onEdit: (s: ExportSchedule) => void;
  onDelete: (s: ExportSchedule) => void;
};

function LastRunCell({ s }: { s: ExportSchedule }) {
  const { t } = useTranslation('instancebackup');
  return (
    <div className="flex items-center gap-2">
      {s.lastStatus ? (
        <Badge variant={s.lastStatus === 'failed' ? 'destructive' : 'success'}>{t(`status.${s.lastStatus}`)}</Badge>
      ) : (
        <span className="text-muted-foreground">-</span>
      )}
      <StatusError error={s.lastError} title={t('schedules.errorTitle')} />
      {s.lastRunAt && <span className="text-xs text-muted-foreground">{formatDate(s.lastRunAt)}</span>}
    </div>
  );
}

function ActionsCell({ s, actions }: { s: ExportSchedule; actions: ScheduleRowActions }) {
  const { t } = useTranslation('instancebackup');
  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('schedules.runNow')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        disabled={actions.running}
        onClick={() => actions.onRun(s)}
      >
        <IconPlayerPlay className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('schedules.edit')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        onClick={() => actions.onEdit(s)}
      >
        <IconPencil className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('actions.delete')}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        onClick={() => actions.onDelete(s)}
      >
        <IconTrash className="size-4" />
      </Button>
    </div>
  );
}

export function useScheduleColumns(actions: ScheduleRowActions): ColumnDef<ExportSchedule, unknown>[] {
  const { t } = useTranslation('instancebackup');
  const { t: tc } = useTranslation('common');
  const { running, onRun, onEdit, onDelete } = actions;

  return useMemo<ColumnDef<ExportSchedule, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (s) => s.name,
        header: t('schedules.name'),
        cell: ({ row }) => (
          <>
            <span className="font-medium text-foreground">{row.original.name}</span>
            {!row.original.enabled && (
              <Badge variant="secondary" className="ml-2">
                {t('schedules.disabled')}
              </Badge>
            )}
          </>
        ),
      },
      {
        id: 'when',
        accessorFn: (s) => `${describeCron(s.cron, tc)} (${s.cron})`,
        header: t('schedules.when'),
        meta: { width: 'xl' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <>
            {describeCron(row.original.cron, tc)}
            <span className="ml-2 font-mono text-xs opacity-70">{row.original.cron}</span>
          </>
        ),
      },
      {
        id: 'next',
        accessorFn: (s) => (s.enabled && s.nextRunAt ? s.nextRunAt : ''),
        header: t('schedules.next'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (row.original.enabled && row.original.nextRunAt ? formatDate(row.original.nextRunAt) : '-'),
      },
      {
        id: 'last',
        accessorFn: (s) => (s.lastStatus ? t(`status.${s.lastStatus}`) : ''),
        header: t('schedules.last'),
        cell: ({ row }) => <LastRunCell s={row.original} />,
      },
      {
        id: 'key',
        accessorFn: (s) => t(`keyKind.${s.keyKind}`),
        header: t('table.key'),
        meta: { width: 'sm', defaultVisible: false } satisfies TableColumnMeta,
      },
      {
        id: 'createdBy',
        accessorFn: (s) => s.createdBy,
        header: t('table.createdBy'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
      },
      {
        id: 'actions',
        header: t('table.actions'),
        enableSorting: false,
        meta: { width: 'md', locked: true, noExport: true, label: t('table.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => <ActionsCell s={row.original} actions={{ running, onRun, onEdit, onDelete }} />,
      },
    ],
    [t, tc, running, onRun, onEdit, onDelete],
  );
}
