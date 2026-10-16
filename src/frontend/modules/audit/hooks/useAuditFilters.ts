// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Every piece of audit filter state, in one place: the search query and its
// match mode, the action and entity-type filters, the date range, and the page
// offset the whole lot invalidates. Extracted from AuditPage so the page itself
// is the TablePage call and the dialogs, nothing more.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DEFAULT_DATE_RANGE_VALUE, type DateRangeFilterValue } from '@resources/components/DateRangeFilter';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import { DATE_RANGE_PRESETS, resolveDateRange } from '@resources/utils/date-range';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import type { ActiveFilterChip } from '@resources/layout/table-page/types';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';

export const AUDIT_FILTERS_DEFAULT_STATE: Record<string, unknown> = {
  actionFilters: [] as string[],
  entityTypeFilter: '',
  query: '',
  mode: 'contains' as SearchMode,
  range: DEFAULT_DATE_RANGE_VALUE.preset === 'custom' ? undefined : DEFAULT_DATE_RANGE_VALUE.preset,
  actor: '',
  severities: [] as string[],
};

export function useAuditFilters() {
  const { t } = useTranslation('common');
  const { t: ta } = useTranslation('audit');
  const [actor, setActor] = useState('');
  const [severities, setSeverities] = useState<string[]>([]);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [actionFilters, setActionFilters] = useState<string[]>([]);
  const [entityTypeFilter, setEntityTypeFilter] = useState('');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<number>(tableDefaultPageSize);

  // Every filter change invalidates the current offset, so send the reader back
  // to the first page rather than onto one the new result set may not have.
  const resetPage = useCallback(() => setPage(1), []);

  const resolvedRange = useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);

  // Every ticked action goes over the wire — the API takes the parameter more
  // than once and matches them as a set, so the pager total stays honest no
  // matter how many are selected.
  const serverActions = actionFilters;

  // The export endpoint takes a single action (its params are a flat object),
  // so it keeps the old rule: push the filter only when one action is ticked.
  const exportAction = actionFilters.length === 1 ? (actionFilters[0] ?? '') : '';

  const toggleAction = useCallback(
    (action: string) => {
      setActionFilters((current) =>
        current.includes(action) ? current.filter((entry) => entry !== action) : [...current, action],
      );
      resetPage();
    },
    [resetPage],
  );

  // A typed page number arrives unvalidated, so clamp it here — the list only
  // ever asks the API for a page that exists.
  const clampPage = useCallback((next: number, totalPages: number) => {
    if (!Number.isFinite(next)) return;
    setPage(Math.min(Math.max(Math.trunc(next), 1), totalPages));
  }, []);

  const clearActions = useCallback(() => {
    setActionFilters([]);
    resetPage();
  }, [resetPage]);

  const clearAll = useCallback(() => {
    setActor('');
    setSeverities([]);
    setActionFilters([]);
    setEntityTypeFilter('');
    setDateRange(DEFAULT_DATE_RANGE_VALUE);
    setQuery('');
    resetPage();
  }, [resetPage]);

  const isPristine = useMemo(
    () =>
      query === '' &&
      actor === '' &&
      severities.length === 0 &&
      mode === 'contains' &&
      actionFilters.length === 0 &&
      entityTypeFilter === '' &&
      dateRange.preset === DEFAULT_DATE_RANGE_VALUE.preset,
    [actionFilters.length, actor, dateRange.preset, entityTypeFilter, mode, query, severities.length],
  );

  const chips = useMemo<ActiveFilterChip[]>(() => {
    const items: ActiveFilterChip[] = [];
    if (actor.trim()) {
      items.push({
        id: 'actor',
        label: ta('filters.actorChip', { actor: actor.trim() }),
        onRemove: () => {
          setActor('');
          resetPage();
        },
      });
    }
    for (const severity of severities) {
      items.push({
        id: `severity:${severity}`,
        label: ta('filters.severityChip', { severity: t(`audit.severity.${severity}`) }),
        onRemove: () => {
          setSeverities((current) => current.filter((entry) => entry !== severity));
          resetPage();
        },
      });
    }
    if (entityTypeFilter) {
      items.push({
        id: 'entityType',
        label: entityTypeFilter,
        onRemove: () => {
          setEntityTypeFilter('');
          resetPage();
        },
      });
    }
    for (const action of actionFilters) {
      items.push({
        id: `action:${action}`,
        label: action,
        onRemove: () => {
          setActionFilters((current) => current.filter((entry) => entry !== action));
          resetPage();
        },
      });
    }
    if (dateRange.preset !== DEFAULT_DATE_RANGE_VALUE.preset) {
      const preset = DATE_RANGE_PRESETS.find((entry) => entry.id === dateRange.preset);
      items.push({
        id: 'range',
        label: preset ? t(preset.i18nKey.replace(/^[^:]+:/, '')) : dateRange.preset,
        onRemove: () => {
          setDateRange(DEFAULT_DATE_RANGE_VALUE);
          resetPage();
        },
      });
    }
    return items;
  }, [actionFilters, actor, dateRange.preset, entityTypeFilter, resetPage, severities, t, ta]);

  return {
    query,
    setQuery,
    mode,
    setMode,
    actor,
    setActor,
    severities,
    setSeverities,
    actionFilters,
    setActionFilters,
    entityTypeFilter,
    setEntityTypeFilter,
    dateRange,
    setDateRange,
    page,
    setPage,
    pageSize,
    setPageSize,
    resolvedRange,
    matcher,
    serverActions,
    exportAction,
    resetPage,
    clampPage,
    toggleAction,
    clearActions,
    clearAll,
    chips,
    isPristine,
  };
}

/** The entity-type dropdown is built from whatever the current page returned. */
export function useEntityTypeOptions(entries: AuditEntry[]) {
  const { t } = useTranslation('common');
  return useMemo(() => {
    const types = new Set<string>();
    for (const entry of entries) {
      if (entry.entityType) types.add(entry.entityType);
    }
    return [
      { value: '', label: t('audit.allEntityTypes') },
      ...[...types].sort().map((et) => ({ value: et, label: et })),
    ];
  }, [entries, t]);
}

/**
 * The action filter and the `contains` / `exact` search modes are applied by
 * the API, so the only thing left to narrow here is `regex`, which has no
 * portable SQL equivalent — and it can only see the page in hand.
 */
export function useFilteredAuditEntries(
  entries: AuditEntry[],
  query: string,
  mode: SearchMode,
  envName: (id: string | null, storedName?: string | null) => string,
) {
  const { t } = useTranslation('common');
  return useMemo(() => {
    if (mode !== 'regex' || !query.trim()) return entries;
    return entries.filter((entry) => {
      return matchesSearchFields(query, mode, [
        entry.timestamp,
        t(`audit.severity.${entry.severity}`),
        entry.username,
        entry.action,
        entry.entityName,
        entry.entityType,
        entry.entityId,
        envName(entry.environmentId, entry.environmentName),
        entry.details,
      ]).matched;
    });
  }, [entries, envName, mode, query, t]);
}
