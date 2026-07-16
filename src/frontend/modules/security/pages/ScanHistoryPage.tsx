// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconBug,
  IconLoader2,
  IconTrash,
} from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@resources/components/ui/Tooltip';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { useDeleteScan, useAllScans, type Scan } from '../../containers/hooks/useScans';

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  pending: 'outline',
  running: 'default',
  completed: 'secondary',
  failed: 'destructive',
};

const STATUS_OPTIONS = [
  { value: '', key: 'scanHistory.status.all' },
  { value: 'pending', key: 'scan.status.pending' },
  { value: 'running', key: 'scan.status.running' },
  { value: 'completed', key: 'scan.status.completed' },
  { value: 'failed', key: 'scan.status.failed' },
] as const;

export default function ScanHistoryPage() {
  const { t } = useTranslation('security');
  const formatPrefs = useLocaleFormat();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');

  const { data, isLoading, isFetching } = useAllScans({ page, perPage: 25, status });
  const deleteScan = useDeleteScan();

  const items = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / (data?.per_page ?? 25)));

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        title={t('scanHistory.title')}
        description={t('scanHistory.description')}
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Select
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={STATUS_OPTIONS.map((opt) => ({
            value: opt.value,
            label: t(opt.key),
          }))}
          ariaLabel={t('scanHistory.status.all')}
          className="max-w-xs"
        />
      </div>

      <div className="flex min-h-0 flex-1 flex-col">
        {isLoading && items.length === 0 ? (
          <div className="flex flex-1 items-center justify-center">
            <Spinner size="lg" />
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center rounded-lg border border-dashed border-border bg-card/40 p-10 text-center">
            <IconBug className="mb-3 size-8 text-muted-foreground" />
            <p className="text-sm font-medium text-foreground">
              {t('scanHistory.emptyTitle')}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {t('scanHistory.emptyHint')}
            </p>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="min-h-0 flex-1 overflow-auto">
              <table className="w-full text-sm">
                <thead className="sticky top-0 z-10 bg-muted">
                  <tr className="text-left text-xs uppercase tracking-wide text-muted-foreground">
                    <th className="px-3 py-3 font-medium">{t('scanHistory.table.time')}</th>
                    <th className="px-3 py-3 font-medium">{t('scanHistory.table.status')}</th>
                    <th className="px-3 py-3 font-medium">{t('scanHistory.table.scanner')}</th>
                    <th className="px-3 py-3 font-medium">{t('scanHistory.table.image')}</th>
                    <th className="px-3 py-3 font-medium text-right">{t('scanHistory.table.severity')}</th>
                    <th className="px-3 py-3 font-medium text-right">{t('scanHistory.table.total')}</th>
                    <th className="px-3 py-3 font-medium" />
                  </tr>
                </thead>
                <tbody>
                  {items.map((scan) => (
                    <ScanRow
                      key={scan.id}
                      scan={scan}
                      formatPrefs={formatPrefs}
                      deleting={deleteScan.isPending}
                      onDelete={() => deleteScan.mutate(scan.id)}
                      labels={{
                        running: t('scanHistory.status.running'),
                        pending: t('scanHistory.status.pending'),
                        completed: t('scanHistory.status.completed'),
                        failed: t('scanHistory.status.failed'),
                        deleteScan: t('scanHistory.table.delete'),
                        viewDetails: t('scanHistory.table.viewDetails'),
                      }}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <div>{t('scanHistory.total', { total })}</div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page <= 1 || isFetching}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t('scanHistory.previous')}
          </Button>
          <span>
            {t('scanHistory.pageOf', { page, totalPages })}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page >= totalPages || isFetching}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            {t('scanHistory.next')}
          </Button>
        </div>
      </div>
    </div>
  );
}

type RowLabels = {
  running: string;
  pending: string;
  completed: string;
  failed: string;
  deleteScan: string;
  viewDetails: string;
};

function ScanRow({
  scan,
  formatPrefs,
  deleting,
  onDelete,
  labels,
}: {
  scan: Scan;
  formatPrefs: { timeFormat: '12h' | '24h'; dateFormat: 'ddmmyyyy' | 'mmddyyyy' };
  deleting: boolean;
  onDelete: () => void;
  labels: RowLabels;
}) {
  const { t } = useTranslation('security');
  const style = severityStyle(scan.severity);
  const statusLabel =
    scan.status === 'pending'
      ? labels.pending
      : scan.status === 'running'
        ? labels.running
        : scan.status === 'completed'
          ? labels.completed
          : scan.status === 'failed'
            ? labels.failed
            : scan.status;
  return (
    <tr className="border-t border-border align-top transition-colors hover:bg-muted/30">
      <td className="px-3 py-3 font-mono text-xs text-muted-foreground whitespace-nowrap">
        {formatDate(scan.completedAt || scan.startedAt, formatPrefs)}
      </td>
      <td className="px-3 py-3">
        {scan.status === 'running' ? (
          <span className="inline-flex items-center gap-1.5">
            <IconLoader2 className="size-3 animate-spin text-muted-foreground" />
            <span className="text-xs font-medium">{statusLabel}</span>
          </span>
        ) : (
          <Badge variant={STATUS_VARIANT[scan.status] ?? 'secondary'}>{statusLabel}</Badge>
        )}
      </td>
      <td className="px-3 py-3 font-mono text-xs">{scan.scanner}</td>
      <td className="px-3 py-3 font-mono text-xs text-muted-foreground">
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="block max-w-xs truncate">{scan.imageRef}</span>
          </TooltipTrigger>
          <TooltipContent>{scan.imageRef}</TooltipContent>
        </Tooltip>
      </td>
      <td className="px-3 py-3 text-right">
        {scan.status === 'completed' ? (
          <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium capitalize ${style.pill} ${style.text}`}>
            <span className={`size-1.5 rounded-full ${style.dot}`} aria-hidden="true" />
            {t(`scan.severity.${scan.severity}`)}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="px-3 py-3 text-right">
        {scan.status === 'completed' && scan.totalVulns > 0 ? (
          <span className="inline-flex gap-1.5">
            {scan.criticalCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                C:{scan.criticalCount}
              </Badge>
            )}
            {scan.highCount > 0 && (
              <Badge variant="destructive" className="text-[10px]">
                H:{scan.highCount}
              </Badge>
            )}
            {scan.mediumCount > 0 && (
              <Badge variant="outline" className="text-[10px]">
                M:{scan.mediumCount}
              </Badge>
            )}
            {scan.lowCount > 0 && (
              <Badge variant="secondary" className="text-[10px]">
                L:{scan.lowCount}
              </Badge>
            )}
          </span>
        ) : scan.status === 'completed' ? (
          <span className="text-xs text-muted-foreground">0</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="px-3 py-3 text-right">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onDelete}
              disabled={deleting}
              aria-label={labels.deleteScan}
            >
              <IconTrash className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{labels.deleteScan}</TooltipContent>
        </Tooltip>
      </td>
    </tr>
  );
}