// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconActivity } from '@tabler/icons-react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { TablePage } from '@resources/layout/TablePage';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { formatDate, truncateId } from '@resources/utils/format';
import { DATE_RANGE_PRESETS, resolveDateRange } from '@resources/utils/date-range';
import type { ActiveFilterChip } from '@resources/layout/table-page/types';
import { useUrlNumberState, useUrlSearchState } from '@resources/hooks/useUrlSearchState';
import { EnvironmentScopeFilter } from '@resources/components/EnvironmentScopeFilter';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import { DegradedStateBanner } from '@resources/components/DegradedStateBanner';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { ViewModeToggle } from '@resources/components/ViewModeToggle';
import { type ContainerEvent, parseMeta } from '../components/EventDetails';
import { ActivityEventDialog } from '../components/ActivityEventDialog';
import { ActivityTimelineView } from '../components/ActivityTimelineView';
import { useActivityViewStore } from '../stores/activity-view';
import { useActivityColumns } from '../components/ActivityColumns';
import { useEventsPage } from '../hooks/useEvents';

const searchModes = ['contains', 'exact', 'regex'] as const;

export default function ActivityPage() {
  const { t } = useTranslation('common');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);
  const resolvedRange = useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);
  const [page, setPage] = useUrlNumberState(1);
  const [pageSize, setPageSize] = useState<number>(tableDefaultPageSize);

  const environments = useEnvironmentStore((s) => s.environments);
  const viewMode = useActivityViewStore((s) => s.viewMode);
  const setViewMode = useActivityViewStore((s) => s.setViewMode);
  const [selected, setSelected] = useState<ContainerEvent | null>(null);
  const [query, setQuery] = useUrlSearchState<string>('');
  const [mode, setMode] = useUrlSearchState<SearchMode>('contains', {
    param: 'mode',
    allowed: searchModes,
  });
  const [scopeEnvId, setScopeEnvId] = useUrlSearchState<string>('', { param: 'scope' });
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);

  // `contains` and `exact` are matched by the API, so the row count below is
  // the real number of matches. `regex` has no portable SQL equivalent, so it
  // alone still narrows the page in hand.
  const { data, isLoading, isFetching, error, failureCount, errorUpdatedAt, refetch } = useEventsPage({
    page,
    perPage: pageSize,
    range: resolvedRange,
    search: query,
    mode,
    envId: scopeEnvId,
  });

  const events = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const totalPages = Math.max(data?.total_pages ?? 1, 1);
  const rangeFrom = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeTo = events.length === 0 ? 0 : rangeFrom + events.length - 1;

  const envName = useCallback(
    (id: string | null) => {
      if (!id) return t('audit.local');
      return environments.find((e) => e.id === id)?.name ?? truncateId(id);
    },
    [environments, t],
  );

  // The API already applied `contains` and `exact`, so only `regex` is left to
  // narrow here — and it can only see the page it was handed.
  const filteredEvents = useMemo(() => {
    if (mode !== 'regex' || !query.trim()) return events;
    return events.filter(
      (eventItem) =>
        matchesSearchFields(query, mode, [
          formatDate(eventItem.timestamp),
          envName(eventItem.environmentId),
          eventItem.action,
          eventItem.containerName,
          eventItem.containerId,
          parseMeta(eventItem.metadata).image,
        ]).matched,
    );
  }, [envName, events, mode, query]);

  const columns = useActivityColumns(envName, setSelected);

  const chips = useMemo<ActiveFilterChip[]>(() => {
    if (dateRange.preset === DEFAULT_DATE_RANGE_VALUE.preset) return [];
    const preset = DATE_RANGE_PRESETS.find((entry) => entry.id === dateRange.preset);
    return [
      {
        id: 'range',
        label: preset ? t(preset.i18nKey.replace(/^[^:]+:/, '')) : dateRange.preset,
        onRemove: () => setDateRange(DEFAULT_DATE_RANGE_VALUE),
      },
    ];
  }, [dateRange.preset, t]);

  const clearAll = useCallback(() => {
    setDateRange(DEFAULT_DATE_RANGE_VALUE);
    setQuery('');
    setPage(1);
  }, [setPage, setQuery]);

  return (
    <>
      <TablePage
        id="activity"
        title={t('activity.title')}
        description={t('activity.description')}
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
          value: query,
          onChange: (value) => {
            setQuery(value);
            setPage(1);
          },
          mode,
          onModeChange: (value) => {
            setMode(value);
            setPage(1);
          },
          placeholder: t('activity.searchPlaceholder'),
          error: matcher.error !== null,
        }}
        savedFilters={{
          // `id` "activity" derives `mcharbor-activity-filters` — the key this
          // page already used, so presets saved earlier still load.
          state: { query, mode, scope: scopeEnvId },
          onRestore: (state) => {
            setQuery(typeof state.query === 'string' ? state.query : '');
            setMode((state.mode as SearchMode) ?? 'contains');
            setScopeEnvId(typeof state.scope === 'string' ? state.scope : '');
          },
          pristine: !query && mode === 'contains' && !scopeEnvId,
          defaults: { query: '', mode: 'contains', scope: '' },
        }}
        filters={
          <>
            <EnvironmentScopeFilter
              value={scopeEnvId}
              onChange={(value) => {
                setScopeEnvId(value);
                setPage(1);
              }}
            />
            <DateRangeFilter
              value={dateRange}
              onChange={(value) => {
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
          export: { filename: 'mcharbor-activity' },
          chips: { items: chips, onClear: clearAll },
        }}
        columns={columns}
        data={filteredEvents}
        loading={isLoading}
        error={error}
        onRetry={() => void refetch()}
        empty={{
          icon: <IconActivity className="size-8" />,
          title: query.trim() ? t('filters.noMatches') : t('activity.noEvents'),
        }}
        getRowId={(evt) => evt.id}
        body={
          viewMode === 'table' ? undefined : (
            <ActivityTimelineView
              events={filteredEvents}
              loading={isLoading}
              envName={envName}
              onSelect={setSelected}
              emptyTitle={query.trim() ? t('filters.noMatches') : t('activity.noEvents')}
            />
          )
        }
        pagination={{
          mode: 'server',
          page,
          totalPages,
          pageSize,
          total,
          from: rangeFrom,
          to: rangeTo,
          onPageChange: (next) => setPage(Math.min(Math.max(Math.trunc(next), 1), totalPages)),
          onPageSizeChange: (next) => {
            setPageSize(next);
            setPage(1);
          },
        }}
      />

      <ActivityEventDialog selected={selected} onClose={() => setSelected(null)} envName={envName} />
    </>
  );
}
