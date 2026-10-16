// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The scan-history table's seven columns, previously the `ScanRow` component's
// hand-written `<td>` markup. Cell rendering is unchanged.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLoader2, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { cn } from '@resources/utils/cn';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { Scan } from '@resources/hooks/useScans';
import { PERM } from '@core/auth/permissionKeys';

const DASH = '—';
const SEVERITY_LETTER = { critical: 'C', high: 'H', medium: 'M', low: 'L' } as const;

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  pending: 'outline',
  running: 'default',
  completed: 'secondary',
  failed: 'destructive',
};

type UseScanHistoryColumnsArgs = {
  deleting: boolean;
  onDelete: (scan: Scan) => void;
};

export function useScanHistoryColumns({ deleting, onDelete }: UseScanHistoryColumnsArgs): ColumnDef<Scan, unknown>[] {
  const { t } = useTranslation('security');
  const formatPrefs = useLocaleFormat();
  const { data: environments } = useEnvironmentList();
  const envNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const env of environments ?? []) map.set(env.id, env.name);
    return map;
  }, [environments]);
  const group = t('columns.groupScan');
  const sev = t('columns.groupSeverity');

  return useMemo(
    () => [
      {
        id: 'date',
        accessorFn: (scan) => formatDate(scan.completedAt || scan.startedAt, formatPrefs),
        header: t('scanHistory.table.date'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs whitespace-nowrap text-muted-foreground">
            {formatDate(row.original.completedAt || row.original.startedAt, formatPrefs)}
          </span>
        ),
      },
      {
        id: 'status',
        accessorFn: (scan) => t(`scanHistory.status.${scan.status}`),
        header: t('scanHistory.table.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const scan = row.original;
          const label = t(`scanHistory.status.${scan.status}`);
          if (scan.status === 'running') {
            return (
              <span className="inline-flex items-center gap-1.5">
                <IconLoader2 className="size-3 animate-spin text-muted-foreground" />
                <span className="text-xs font-medium">{label}</span>
              </span>
            );
          }
          return <Badge variant={STATUS_VARIANT[scan.status] ?? 'secondary'}>{label}</Badge>;
        },
      },
      {
        id: 'scanner',
        accessorFn: (scan) => scan.scanner,
        header: t('scanHistory.table.scanner'),
        cell: ({ row }) => <span className="font-mono text-xs">{row.original.scanner}</span>,
      },
      {
        id: 'image',
        accessorFn: (scan) => scan.imageRef,
        header: t('scanHistory.table.image'),
        cell: ({ row }) => (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="block max-w-xs truncate font-mono text-xs text-muted-foreground">
                {row.original.imageRef}
              </span>
            </TooltipTrigger>
            <TooltipContent>{row.original.imageRef}</TooltipContent>
          </Tooltip>
        ),
      },
      {
        id: 'severity',
        accessorFn: (scan) => (scan.status === 'completed' ? scan.severity : ''),
        header: t('scanHistory.table.severity'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const scan = row.original;
          if (scan.status !== 'completed') return <span className="text-muted-foreground">{DASH}</span>;
          const style = severityStyle(scan.severity);
          return (
            <span
              className={cn(
                'inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-medium capitalize',
                style.pill,
                style.text,
              )}
            >
              <span className={cn('size-1.5 rounded-full', style.dot)} aria-hidden="true" />
              {t(`scan.severity.${scan.severity}`)}
            </span>
          );
        },
      },
      {
        id: 'vulns',
        accessorFn: (scan) => (scan.status === 'completed' ? scan.totalVulns : ''),
        header: t('scanHistory.table.vulnerabilities'),
        cell: ({ row }) => {
          const scan = row.original;
          if (scan.status !== 'completed') return <span className="text-muted-foreground">{DASH}</span>;
          if (scan.totalVulns === 0) return <span className="text-xs text-muted-foreground">0</span>;
          return (
            <span className="inline-flex gap-1.5">
              {scan.criticalCount > 0 && (
                <Badge variant="destructive" className="text-[10px]">
                  {SEVERITY_LETTER.critical}:{scan.criticalCount}
                </Badge>
              )}
              {scan.highCount > 0 && (
                <Badge variant="destructive" className="text-[10px]">
                  {SEVERITY_LETTER.high}:{scan.highCount}
                </Badge>
              )}
              {scan.mediumCount > 0 && (
                <Badge variant="outline" className="text-[10px]">
                  {SEVERITY_LETTER.medium}:{scan.mediumCount}
                </Badge>
              )}
              {scan.lowCount > 0 && (
                <Badge variant="secondary" className="text-[10px]">
                  {SEVERITY_LETTER.low}:{scan.lowCount}
                </Badge>
              )}
            </span>
          );
        },
      },
      textColumn<Scan>('scanId', t('columns.id'), (r) => r.id, { group }),
      textColumn<Scan>(
        'environment',
        t('columns.environment'),
        (r) => r.environmentName ?? envNameById.get(r.environmentId),
        { group },
      ),
      dateColumn<Scan>('startedAt', t('columns.startedAt'), (r) => r.startedAt, { group }),
      dateColumn<Scan>('completedAt', t('columns.completedAt'), (r) => r.completedAt, { group }),
      textColumn<Scan>('critical', t('columns.critical'), (r) => r.criticalCount, { group: sev, width: 'xs' }),
      textColumn<Scan>('high', t('columns.high'), (r) => r.highCount, { group: sev, width: 'xs' }),
      textColumn<Scan>('medium', t('columns.medium'), (r) => r.mediumCount, { group: sev, width: 'xs' }),
      textColumn<Scan>('low', t('columns.low'), (r) => r.lowCount, { group: sev, width: 'xs' }),
      {
        id: 'actions',
        header: t('labels.actions', { ns: 'common' }),
        enableSorting: false,
        meta: {
          width: 'xs',
          locked: true,
          noExport: true,
          label: t('labels.actions', { ns: 'common' }),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  permission={PERM.SCANS_MANAGE}
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onDelete(row.original)}
                  disabled={deleting}
                  aria-label={t('scanHistory.table.delete')}
                >
                  <IconTrash className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('scanHistory.table.delete')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [deleting, formatPrefs, onDelete, t, group, sev, envNameById],
  );
}
