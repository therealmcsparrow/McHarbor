// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHistory } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { DEFAULT_DATE_RANGE_VALUE, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { useUrlNumberState, useUrlSearchState } from '@resources/hooks/useUrlSearchState';
import { DegradedStateBanner } from '@resources/components/DegradedStateBanner';
import { EnvironmentScopeFilter } from '@resources/components/EnvironmentScopeFilter';
import { ViewModeToggle } from '@resources/components/ViewModeToggle';
import {
  useLifecycleEvents,
  type LifecycleEvent,
  type LifecycleListParams,
  type LifecycleSubjectType,
  type LifecycleSeverity,
} from '../hooks/useLifecycle';
import { useLifecycleColumns } from '../components/LifecycleColumns';
import { LifecycleFilterControls } from '../components/LifecycleFilterControls';
import { severityValues } from '../components/lifecycle-filter-options';
import { useLifecycleSubjectTypes } from '../hooks/useLifecycleSubjectTypes';
import { LifecycleDetailsDialog } from '../components/LifecycleDetailsDialog';
import { LifecycleTimelineView } from '../components/LifecycleTimelineView';
import { useLifecycleViewStore } from '../stores/lifecycle-view';
import { useLifecycleFilterChips, useResolvedDateRange } from './useLifecycleFilterChips';
import { useLifecycleSavedFilters } from './useLifecycleSavedFilters';

const PER_PAGE = 50;

export default function LifecycleLogPage() {
  const { t } = useTranslation('settings');
  const [envId, setEnvId] = useUrlSearchState<string>('', { param: 'scope' });
  const environments = useEnvironmentStore((s) => s.environments);
  const viewMode = useLifecycleViewStore((s) => s.viewMode);
  const setViewMode = useLifecycleViewStore((s) => s.setViewMode);

  const subjectOptions = useLifecycleSubjectTypes();
  const subjectTypeValues = useMemo(() => subjectOptions.map((o) => o.value), [subjectOptions]);
  const [subjectType, setSubjectType] = useUrlSearchState<LifecycleSubjectType>('', {
    param: 'subject',
    allowed: subjectTypeValues,
  });
  const [severity, setSeverity] = useUrlSearchState<LifecycleSeverity | ''>('', {
    param: 'severity',
    allowed: severityValues,
  });
  const [search, setSearch] = useUrlSearchState<string>('');
  const [page, setPage] = useUrlNumberState(1);
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);
  const [selected, setSelected] = useState<LifecycleEvent | null>(null);

  const resolvedRange = useResolvedDateRange(dateRange);

  // The API does the filtering — subject, severity, text and range all go over
  // the wire — so TablePage receives rows that are already narrowed.
  const params = useMemo<LifecycleListParams>(
    () => ({
      page,
      perPage: PER_PAGE,
      envId: envId === '' ? undefined : envId,
      subjectType,
      severity,
      from: resolvedRange.from?.toISOString(),
      // The API names the upper bound `until`, not `to`.
      until: resolvedRange.to?.toISOString(),
      search,
    }),
    [page, envId, subjectType, severity, search, resolvedRange.from, resolvedRange.to],
  );

  const { data, isLoading, isFetching, refetch, error, failureCount, errorUpdatedAt } = useLifecycleEvents(params);
  const formatPrefs = useLocaleFormat();
  const columns = useLifecycleColumns({ formatPrefs, environments, onSelect: setSelected });

  const items = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const perPage = data?.perPage ?? PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const from = total === 0 ? 0 : (page - 1) * perPage + 1;
  const to = items.length === 0 ? 0 : from + items.length - 1;

  const savedFilters = useLifecycleSavedFilters({
    envId,
    setEnvId,
    subjectType,
    setSubjectType,
    severity,
    setSeverity,
    search,
    setSearch,
    dateRange,
    setDateRange,
    setPage,
  });

  const chips = useLifecycleFilterChips({
    subjectType,
    setSubjectType,
    severity,
    setSeverity,
    dateRange,
    setDateRange,
    setPage,
    t,
  });

  return (
    <>
      <TablePage
        id="lifecycle"
        title={t('lifecycle.title')}
        description={t('lifecycle.description')}
        actions={<ViewModeToggle value={viewMode} onChange={setViewMode} />}
        banner={
          <DegradedStateBanner
            error={error}
            failureCount={failureCount}
            updatedAt={errorUpdatedAt}
            onRetry={() => void refetch()}
          />
        }
        search={{
          value: search,
          onChange: (value) => {
            setSearch(value);
            setPage(1);
          },
          // The API matches substrings only; offering exact/regex here would
          // promise three modes and deliver one.
          mode: 'contains',
          onModeChange: () => undefined,
          placeholder: t('lifecycle.searchPlaceholder'),
        }}
        savedFilters={savedFilters}
        filters={
          <>
            <EnvironmentScopeFilter
              value={envId}
              onChange={(value) => {
                setEnvId(value);
                setPage(1);
              }}
            />
            <LifecycleFilterControls
              subjectType={subjectType}
              onSubjectTypeChange={(value) => {
                setSubjectType(value);
                setPage(1);
              }}
              severity={severity}
              onSeverityChange={(value) => {
                setSeverity(value);
                setPage(1);
              }}
              dateRange={dateRange}
              onDateRangeChange={(value) => {
                setDateRange(value);
                setPage(1);
              }}
            />
          </>
        }
        toolbar={{
          // Column and density controls act on the table; the timeline has
          // neither columns nor row height to steer.
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-lifecycle' },
          chips: {
            items: chips,
            onClear: () => {
              setSubjectType('');
              setSeverity('');
              setDateRange(DEFAULT_DATE_RANGE_VALUE);
              setSearch('');
              setPage(1);
            },
          },
        }}
        columns={columns}
        data={items}
        loading={isLoading && items.length === 0}
        empty={{ icon: <IconHistory className="size-8" />, title: t('lifecycle.empty') }}
        getRowId={(evt) => evt.id}
        onRowClick={setSelected}
        body={
          viewMode === 'table' ? undefined : (
            <LifecycleTimelineView
              events={items}
              loading={isLoading}
              formatPrefs={formatPrefs}
              environments={environments}
              onSelect={setSelected}
              emptyTitle={t('lifecycle.empty')}
            />
          )
        }
        pagination={{
          mode: 'server',
          page,
          totalPages,
          pageSize: perPage,
          total,
          from,
          to,
          onPageChange: setPage,
          // Page size is fixed by the API call this page makes.
          onPageSizeChange: () => undefined,
          pageSizeOptions: [PER_PAGE],
        }}
      />

      <LifecycleDetailsDialog event={selected} onOpenChange={(isOpen) => !isOpen && setSelected(null)} />
    </>
  );
}
