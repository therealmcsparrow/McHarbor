// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useEnvironmentStore } from '@resources/stores/environment';
import { SearchFilterToolbar } from '@resources/components/SearchFilterToolbar';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { resolveDateRange } from '@resources/utils/date-range';
import { useBackupLogs, type BackupLogSeverity } from '../hooks/useBackupLogs';
import { BackupLogRow, BackupLogPagination } from './BackupLogRow';

const SEVERITY_OPTIONS: { value: BackupLogSeverity | ''; key: string }[] = [
  { value: '', key: 'backups.logs.severity.all' },
  { value: 'info', key: 'backups.logs.severity.info' },
  { value: 'success', key: 'backups.logs.severity.success' },
  { value: 'warning', key: 'backups.logs.severity.warning' },
  { value: 'error', key: 'backups.logs.severity.error' },
];

export default function BackupLogsTab() {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);
  const [page, setPage] = useState(1);
  const [severity, setSeverity] = useState<BackupLogSeverity | ''>('');
  const [search, setSearch] = useState('');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);

  const resolvedRange = useMemo(
    () => resolveDateRange(dateRange.preset, dateRange.custom),
    [dateRange],
  );

  const params = useMemo(
    () => ({
      envId: envId === '' ? undefined : envId,
      severity,
      search,
      from: resolvedRange.from?.toISOString(),
      to: resolvedRange.to?.toISOString(),
      page,
      perPage: 50,
    }),
    [envId, severity, search, resolvedRange.from, resolvedRange.to, page],
  );

  const { data, isLoading, isFetching } = useBackupLogs(params);
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / (data?.perPage ?? 50)));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <SearchFilterToolbar
        query={search}
        onQueryChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        mode={'contains'}
        onModeChange={() => undefined}
        placeholder={t('backups.logs.searchPlaceholder')}
        extraControls={
          <div className="flex flex-col gap-3 md:flex-row md:items-center">
            <Select
              value={severity}
              onChange={(v) => {
                setSeverity(v as BackupLogSeverity | '');
                setPage(1);
              }}
              options={SEVERITY_OPTIONS.map((opt) => ({
                value: opt.value,
                label: t(opt.key),
              }))}
              ariaLabel={t('backups.logs.severity.all')}
            />
            <DateRangeFilter
              value={dateRange}
              onChange={(v) => {
                setDateRange(v);
                setPage(1);
              }}
            />
          </div>
        }
      />

      <div className="flex min-h-0 flex-1 flex-col">
        {isLoading && items.length === 0 ? (
          <div className="flex flex-1 items-center justify-center">
            <Spinner size="lg" />
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-1 items-center justify-center rounded-lg border border-dashed border-border bg-card/40 p-10 text-center">
            <p className="text-sm font-medium text-foreground">
              {t('backups.logs.emptyTitle')}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {t('backups.logs.emptyHint')}
            </p>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-card">
            <div className="min-h-0 flex-1 overflow-auto">
              <table className="w-full text-sm">
                <thead className="sticky top-0 z-10 bg-muted">
                  <tr className="text-left text-xs uppercase tracking-wide text-muted-foreground">
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.time')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.severity')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.action')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.message')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.container')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.plan')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.environment')}</th>
                    <th className="px-3 py-3 font-medium">{t('backups.logs.table.phase')}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((log) => (
                    <BackupLogRow key={log.id} log={log} />
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        <BackupLogPagination
          page={page}
          totalPages={totalPages}
          total={total}
          disabled={isFetching}
          onPageChange={setPage}
          labels={{
            total: t('backups.logs.total'),
            pageOf: t('backups.logs.pageOf'),
          }}
        />
      </div>
    </div>
  );
}