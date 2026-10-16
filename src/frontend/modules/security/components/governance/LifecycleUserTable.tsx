// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLockOpen, IconTrash, IconUserCheck, IconUserOff, IconUsers } from '@tabler/icons-react';
import type { ComponentType } from 'react';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { StatusTone } from '@resources/theme/status-palette';
import { TablePage } from '@resources/layout/TablePage';
import type { BatchAction } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { PERM } from '@core/auth/permissionKeys';
import {
  LIFECYCLE_FILTERS,
  matchesLifecycleFilter,
  type BulkAction,
  type LifecycleCounts,
  type LifecycleFilter,
  type LifecycleUser,
} from '../../hooks/useUserLifecycle';
import { lifecycleSourceLabel, useLifecycleUserColumns } from './LifecycleUserColumns';

const ACTIONS: { action: BulkAction; permission: string; icon: ComponentType<{ className?: string }> }[] = [
  { action: 'disable', permission: PERM.ACCOUNT_MANAGE, icon: IconUserOff },
  { action: 'enable', permission: PERM.ACCOUNT_MANAGE, icon: IconUserCheck },
  { action: 'unlock', permission: PERM.ACCOUNT_MANAGE, icon: IconLockOpen },
  { action: 'delete', permission: PERM.USERS_MANAGE, icon: IconTrash },
];

const FACET_TONE: Partial<Record<LifecycleFilter, StatusTone>> = {
  locked: 'warning',
  noMfa: 'warning',
  recentlyElevated: 'info',
};

const ALL_SOURCES = 'all';

export function useLifecycleTableFilters() {
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<LifecycleFilter>('all');
  const [source, setSource] = useState(ALL_SOURCES);
  return { query, setQuery, mode, setMode, facet, setFacet, source, setSource };
}

export type LifecycleTableFilters = ReturnType<typeof useLifecycleTableFilters>;

type LifecycleUserTableProps = {
  users: LifecycleUser[];
  counts?: LifecycleCounts;
  loading: boolean;
  fetching: boolean;
  error: unknown;
  onRefresh: () => void;
  filters: LifecycleTableFilters;
  inactiveDays: number;
  onInactiveDaysChange: (days: number) => void;
  onBulkAction: (action: BulkAction, users: LifecycleUser[]) => void;
};

export function LifecycleUserTable({
  users,
  counts,
  loading,
  fetching,
  error,
  onRefresh,
  filters,
  inactiveDays,
  onInactiveDaysChange,
  onBulkAction,
}: LifecycleUserTableProps) {
  const { t } = useTranslation('security');
  const columns = useLifecycleUserColumns();
  const { facet, source } = filters;

  const sources = useMemo(() => [...new Set(users.map((u) => u.authProvider))].sort(), [users]);
  const rows = useMemo(
    () =>
      users.filter((u) => matchesLifecycleFilter(u, facet) && (source === ALL_SOURCES || u.authProvider === source)),
    [users, facet, source],
  );

  const facets = useMemo<FilterFacet<string>[]>(
    () =>
      LIFECYCLE_FILTERS.map((f) => ({
        id: f,
        label: f === 'all' ? t('labels.all', { ns: 'common' }) : t(`governance.lifecycle.badges.${f}`),
        count: (f === 'all' ? counts?.total : counts?.[f]) ?? 0,
        tone: FACET_TONE[f],
      })),
    [counts, t],
  );

  const batchActions = useMemo<BatchAction[]>(
    () =>
      ACTIONS.map(({ action, permission, icon }) => ({
        label: t(`governance.lifecycle.actions.${action}`),
        icon,
        permission,
        variant: action === 'delete' ? 'destructive' : 'default',
        onClick: (selected) => onBulkAction(action, selected as LifecycleUser[]),
      })),
    [onBulkAction, t],
  );

  const sourceChips =
    source === ALL_SOURCES
      ? []
      : [
          {
            id: 'source',
            label: t('governance.lifecycle.sourceChip', { source: lifecycleSourceLabel(source, t) }),
            onRemove: () => filters.setSource(ALL_SOURCES),
          },
        ];

  return (
    <TablePage
      id="user-lifecycle"
      header={false}
      title={t('governance.lifecycle.title')}
      search={{
        value: filters.query,
        onChange: filters.setQuery,
        mode: filters.mode,
        onModeChange: filters.setMode,
        placeholder: t('governance.lifecycle.searchPlaceholder'),
        fields: (u) => [u.displayName, u.username, u.authProvider],
      }}
      savedFilters={{
        state: { query: filters.query, facet, source },
        onRestore: (state) => {
          filters.setQuery(typeof state.query === 'string' ? state.query : '');
          filters.setFacet(
            LIFECYCLE_FILTERS.includes(state.facet as LifecycleFilter) ? (state.facet as LifecycleFilter) : 'all',
          );
          filters.setSource(typeof state.source === 'string' ? state.source : ALL_SOURCES);
        },
        pristine: !filters.query && facet === 'all' && source === ALL_SOURCES,
        defaults: { query: '', facet: 'all', source: ALL_SOURCES },
      }}
      facets={{ items: facets, value: facet, onChange: (next) => filters.setFacet(next as LifecycleFilter) }}
      filters={
        <>
          <Select
            value={source}
            onChange={filters.setSource}
            ariaLabel={t('governance.lifecycle.columns.source')}
            searchable={false}
            options={[
              { value: ALL_SOURCES, label: t('governance.lifecycle.sources.all') },
              ...sources.map((s) => ({ value: s, label: lifecycleSourceLabel(s, t) })),
            ]}
            className="w-40"
          />
          <label className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            {t('governance.lifecycle.inactiveDays')}
            <NumberInput
              size="sm"
              min={1}
              className="w-32"
              value={inactiveDays}
              onChange={(next) => onInactiveDaysChange(Math.max(1, Math.trunc(next)))}
            />
          </label>
        </>
      }
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh, isFetching: fetching },
        export: { filename: 'mcharbor-user-lifecycle' },
        chips: { items: sourceChips, onClear: () => filters.setSource(ALL_SOURCES) },
      }}
      columns={columns}
      data={rows}
      loading={loading}
      error={error}
      onRetry={onRefresh}
      empty={{ icon: <IconUsers className="size-8" />, title: t('governance.lifecycle.empty') }}
      selectable
      batchActions={batchActions}
      getRowId={(u) => u.id}
      getRowTestId={(u) => `lifecycle-user-${u.id}`}
    />
  );
}
