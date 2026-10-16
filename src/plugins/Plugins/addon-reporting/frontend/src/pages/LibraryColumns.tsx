// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The library's columns, as TanStack column defs.
//
// These were hand-written `<th>`/`<td>` markup at the bottom of LibraryPage.
// Moving them here is what lets the page sit on the host's shared TablePage
// layout — and what gives the columns menu, the density control and CSV/JSON
// export something to work with. Cell rendering is unchanged from the original
// markup.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileTypeDocx, IconFileTypePdf, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { cn } from '@resources/utils/cn';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { formatDateWith } from '../date-format';
import { bandStatus, statusPill } from '../blocks/tokens';
import { RowAction } from './RowAction';
import type { StoredReport } from '../types';

type Handlers = {
  onDownload: (report: StoredReport, format: 'pdf' | 'docx') => void;
  onDelete: (report: StoredReport) => void;
};

export function useLibraryColumns({ onDownload, onDelete }: Handlers): ColumnDef<StoredReport, unknown>[] {
  const { t } = useTranslation('reporting');

  return useMemo(
    () => [
      {
        id: 'report',
        accessorFn: (report) => report.title,
        header: t('library.col.report'),
        meta: { width: 'xl' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex min-w-0 flex-col">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium text-foreground">{row.original.title}</span>
              {/* A retired type no longer has a catalog entry to regenerate from; the stored document is still the record. */}
              {row.original.status === 'retired' && (
                <span
                  className={cn(
                    'shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold whitespace-nowrap',
                    statusPill.muted,
                  )}
                  title={t('library.retiredHint')}
                >
                  {t('library.retired')}
                </span>
              )}
            </span>
            <span className="truncate text-xs text-muted-foreground">
              {row.original.environment} · {row.original.headline}
            </span>
          </div>
        ),
      },
      {
        id: 'period',
        accessorFn: (report) => formatPeriod(report),
        header: t('library.col.period'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-muted-foreground">{formatPeriod(row.original)}</span>
        ),
      },
      {
        id: 'score',
        accessorFn: (report) => report.overall_score,
        header: t('library.col.score'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.overall_score >= 0 ? (
            <span
              className={cn(
                'inline-block rounded-full px-2 py-0.5 text-xs font-semibold whitespace-nowrap',
                statusPill[bandStatus(row.original.overall_band)],
              )}
            >
              {row.original.overall_score} · {t(`band.${row.original.overall_band}`)}
            </span>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      },
      {
        id: 'generated',
        accessorFn: (report) => formatDate(report.created_at),
        header: t('library.col.generated'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="whitespace-nowrap text-muted-foreground">{formatDate(row.original.created_at)}</span>
            <span className="truncate text-xs text-muted-foreground">{row.original.generated_by}</span>
          </div>
        ),
      },
      {
        id: 'actions',
        header: t('library.col.actions'),
        enableSorting: false,
        // Icon buttons have no text form, so this column stays out of exports;
        // `locked` also keeps it out of reach of the columns menu.
        meta: { width: 'sm', locked: true, noExport: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <RowAction
              icon={<IconFileTypePdf className="size-3.5" />}
              label={t('action.exportPdf')}
              onClick={() => onDownload(row.original, 'pdf')}
            />
            <RowAction
              icon={<IconFileTypeDocx className="size-3.5" />}
              label={t('action.exportDocx')}
              onClick={() => onDownload(row.original, 'docx')}
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
    [onDownload, onDelete, t],
  );
}

export function formatDate(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '—' : formatDateWith(d, { dateStyle: 'medium' });
}

export function formatPeriod(report: StoredReport): string {
  const from = new Date(report.period_start);
  const to = new Date(report.period_end);
  if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return '—';
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' };
  if (Math.abs(to.getTime() - from.getTime()) < 60_000) {
    return formatDateWith(to, { dateStyle: 'medium' });
  }
  return `${formatDateWith(from, opts)} – ${formatDateWith(to, { ...opts, year: 'numeric' })}`;
}
