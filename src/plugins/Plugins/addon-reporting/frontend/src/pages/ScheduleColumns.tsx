// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The schedules table, as TanStack column defs.
//
// The same shape the library's columns take, for the same reason: sitting
// on the host's shared `TablePage` is what gives the page a columns menu,
// a density control and export without any of it being written here.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconAlertTriangle,
  IconMailFast,
  IconPencil,
  IconPlayerPlay,
  IconServer2,
  IconTrash,
} from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { cn } from '@resources/utils/cn';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { formatDateWith } from '../date-format';
import { statusPill } from '../blocks/tokens';
import { RowAction } from './RowAction';
import type { Schedule, ScheduleRunStatus } from '../types';
import { useTypeName } from './catalog-labels';

type Handlers = {
  onRunNow: (schedule: Schedule) => void;
  onEdit: (schedule: Schedule) => void;
  onDelete: (schedule: Schedule) => void;
  runningId: string | null;
};

/**
 * Run status to the shared status vocabulary.
 *
 * `partial` maps to warn rather than to bad on purpose: some targets did
 * receive the report, and colouring that the same as a total failure would
 * send an operator looking for a problem that is smaller than it appears.
 */
export function runStatusTone(statusValue: ScheduleRunStatus | undefined) {
  switch (statusValue) {
    case 'success':
      return 'good' as const;
    case 'partial':
      return 'warn' as const;
    case 'failed':
      return 'bad' as const;
    case 'running':
      return 'info' as const;
    default:
      return 'muted' as const;
  }
}

export function useScheduleColumns({
  onRunNow,
  onEdit,
  onDelete,
  runningId,
}: Handlers): ColumnDef<Schedule, unknown>[] {
  const { t } = useTranslation('reporting');
  // A schedule row holds a bare type key, so the name is resolved
  // through the catalog — a contributed type would otherwise render as
  // its internal identifier.
  const typeName = useTypeName();

  return useMemo(
    () => [
      {
        id: 'schedule',
        accessorFn: (schedule) => schedule.name,
        header: t('schedules.col.schedule'),
        meta: { width: 'xl' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex min-w-0 flex-col">
            <span className="truncate font-medium text-foreground">{row.original.name}</span>
            <span className="truncate text-xs text-muted-foreground">
              {typeName(row.original.report_type)}
              {row.original.environment ? ` · ${row.original.environment}` : ''}
            </span>
          </div>
        ),
      },
      {
        id: 'cadence',
        accessorFn: (schedule) => schedule.cron,
        header: t('schedules.col.cadence'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="font-mono text-xs whitespace-nowrap text-foreground">{row.original.cron}</span>
            <span className="text-xs whitespace-nowrap text-muted-foreground">
              {row.original.enabled
                ? t('schedules.nextRun', { when: formatDateTime(row.original.next_run_at) })
                : t('schedules.disabled')}
            </span>
          </div>
        ),
      },
      {
        id: 'targets',
        accessorFn: (schedule) => schedule.targets.length,
        header: t('schedules.col.targets'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => <TargetSummary schedule={row.original} />,
      },
      {
        id: 'lastRun',
        accessorFn: (schedule) => schedule.last_run?.status ?? '',
        header: t('schedules.col.lastRun'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const run = row.original.last_run;
          if (!run) return <span className="text-muted-foreground">{t('schedules.neverRun')}</span>;
          return (
            <div className="flex flex-col items-start gap-1">
              <span
                className={cn(
                  'rounded-full px-2 py-0.5 text-xs font-semibold whitespace-nowrap',
                  statusPill[runStatusTone(run.status)],
                )}
              >
                {t(`schedules.status.${run.status}`)}
              </span>
              <span className="text-xs whitespace-nowrap text-muted-foreground">
                {formatDateTime(run.finished_at || run.started_at)}
              </span>
            </div>
          );
        },
      },
      {
        id: 'actions',
        header: t('schedules.col.actions'),
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <RowAction
              icon={<IconPlayerPlay className="size-3.5" />}
              label={t('schedules.runNow')}
              permission="reporting.generate"
              disabled={runningId === row.original.id}
              onClick={() => onRunNow(row.original)}
            />
            <RowAction
              icon={<IconPencil className="size-3.5" />}
              label={t('schedules.edit.action')}
              permission="reporting.manage"
              onClick={() => onEdit(row.original)}
            />
            <RowAction
              icon={<IconTrash className="size-3.5" />}
              label={t('action.delete')}
              permission="reporting.manage"
              danger
              onClick={() => onDelete(row.original)}
            />
          </div>
        ),
      },
    ],
    [onDelete, onEdit, onRunNow, runningId, t, typeName],
  );
}

/**
 * Where this schedule delivers.
 *
 * A target whose host row has gone is marked rather than dropped: a
 * schedule quietly delivering to fewer places than it lists is exactly
 * the failure an operator would never notice on their own.
 */
function TargetSummary({ schedule }: { schedule: Schedule }) {
  const { t } = useTranslation('reporting');
  if (schedule.targets.length === 0) {
    return <span className="text-muted-foreground">—</span>;
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {schedule.targets.map((target) => (
        <span
          key={target.id}
          className={cn(
            'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs',
            target.missing ? 'bg-danger/10 text-danger' : 'bg-muted text-muted-foreground',
          )}
        >
          {target.kind === 'storage' ? (
            <IconServer2 className="size-3" aria-hidden="true" />
          ) : (
            <IconMailFast className="size-3" aria-hidden="true" />
          )}
          <span className="max-w-[10rem] truncate">{target.name || target.ref_id}</span>
          {/* The folder matters when two schedules write to one location:
              without it the chip says where, but not where within. */}
          {target.kind === 'storage' && target.path ? (
            <span className="max-w-[10rem] truncate font-mono opacity-70">/{target.path}</span>
          ) : null}
          {target.missing ? <IconAlertTriangle className="size-3" aria-label={t('schedules.targetMissing')} /> : null}
        </span>
      ))}
    </div>
  );
}

export function formatDateTime(value: string | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '—' : formatDateWith(d, { dateStyle: 'medium', timeStyle: 'short' });
}
