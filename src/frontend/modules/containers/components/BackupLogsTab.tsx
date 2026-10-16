// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { Select } from '@resources/components/ui/Select';
import { resolveDateRange, EMPTY_DATE_RANGE, type DateRangePresetId } from '@resources/utils/date-range';
import { useBackupLogs, type BackupLogSeverity } from '../hooks/useBackupLogs';
import { useBackupLogColumns } from './BackupLogColumns';

const PER_PAGE = 50;

const SEVERITY_OPTIONS: { value: BackupLogSeverity | ''; key: string }[] = [
  { value: '', key: 'backups.logs.severity.all' },
  { value: 'info', key: 'backups.logs.severity.info' },
  { value: 'success', key: 'backups.logs.severity.success' },
  { value: 'warning', key: 'backups.logs.severity.warning' },
  { value: 'error', key: 'backups.logs.severity.error' },
];

export default function BackupLogsTab({ envId }: { envId: string }) {
  const { t } = useTranslation('containers');
  const [page, setPage] = useState(1);
  const [severity, setSeverity] = useState<BackupLogSeverity | ''>('');
  const [search, setSearch] = useState('');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);

  const resolvedRange = useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);

  // The API does the filtering here — severity, text and range all go over the
  // wire — so TablePage receives rows that are already narrowed.
  const params = useMemo(
    () => ({
      envId: envId === '' ? undefined : envId,
      severity,
      search,
      from: resolvedRange.from?.toISOString(),
      to: resolvedRange.to?.toISOString(),
      page,
      perPage: PER_PAGE,
    }),
    [envId, severity, search, resolvedRange.from, resolvedRange.to, page],
  );

  const { data, isLoading, isFetching, refetch } = useBackupLogs(params);
  const columns = useBackupLogColumns();
  const items = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const perPage = data?.perPage ?? PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const from = total === 0 ? 0 : (page - 1) * perPage + 1;
  const to = items.length === 0 ? 0 : from + items.length - 1;

  return (
    <TablePage
      id="backup-logs"
      // Rendered inside the backups tab, which already carries its own heading.
      header={false}
      title={t('backups.logs.tabTitle')}
      search={{
        value: search,
        onChange: (value) => {
          setSearch(value);
          setPage(1);
        },
        // The API matches substrings only, so offering exact/regex here would
        // promise three modes and deliver one.
        mode: 'contains',
        onModeChange: () => undefined,
        placeholder: t('backups.logs.searchPlaceholder'),
      }}
      savedFilters={{
        state: {
          severity,
          search,
          range: dateRange.preset === 'custom' ? undefined : dateRange.preset,
        },
        onRestore: (state) => {
          setSeverity((state.severity as BackupLogSeverity | '') ?? '');
          setSearch(typeof state.search === 'string' ? state.search : '');
          if (typeof state.range === 'string' && state.range !== 'custom') {
            setDateRange({ preset: state.range as DateRangePresetId, custom: EMPTY_DATE_RANGE });
          } else {
            setDateRange(DEFAULT_DATE_RANGE_VALUE);
          }
          setPage(1);
        },
        pristine: !severity && !search,
        defaults: { severity: '', search: '', range: DEFAULT_DATE_RANGE_VALUE.preset },
      }}
      filters={
        <>
          <DateRangeFilter
            value={dateRange}
            onChange={(value) => {
              setDateRange(value);
              setPage(1);
            }}
          />
          <Select
            value={severity}
            onChange={(value) => {
              setSeverity(value as BackupLogSeverity | '');
              setPage(1);
            }}
            options={SEVERITY_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.key) }))}
            ariaLabel={t('backups.logs.severity.all')}
            className="w-44"
          />
        </>
      }
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: { filename: 'mcharbor-backup-logs' },
      }}
      columns={columns}
      data={items}
      loading={isLoading && items.length === 0}
      empty={{ title: t('backups.logs.emptyTitle'), description: t('backups.logs.emptyHint') }}
      getRowId={(log) => log.id}
      pagination={{
        mode: 'server',
        page,
        totalPages,
        pageSize: perPage,
        total,
        from,
        to,
        onPageChange: setPage,
        // Page size is fixed by the API call this tab makes.
        onPageSizeChange: () => undefined,
        pageSizeOptions: [PER_PAGE],
      }}
    />
  );
}
