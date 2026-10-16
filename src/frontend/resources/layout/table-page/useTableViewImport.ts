// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useQueryClient, type QueryClient, type UseQueryResult } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { loadSearchPresets } from '@resources/utils/search-filter';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import { resolveLayout, toLayoutColumns, type DeclaredColumn, type LayoutPayload } from './table-layout';
import { tableViewKey, type TableViewData } from './useTableViewApi';
import { TABLE_DENSITIES, type TableDensity } from './types';

type LegacyPreferences = { hiddenColumns?: unknown; density?: unknown; autoRefreshMs?: unknown; pageSize?: unknown };

const MAX_PRESET_NAME_LENGTH = 80;

function readLegacy(key: string): LegacyPreferences | null {
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as LegacyPreferences) : null;
  } catch {
    return null;
  }
}

function removeKeys(keys: string[]) {
  for (const key of keys) {
    try {
      window.localStorage.removeItem(key);
    } catch {
      return;
    }
  }
}

function legacyLayout(declared: DeclaredColumn[], legacy: LegacyPreferences): LayoutPayload {
  const hidden = new Set(
    Array.isArray(legacy.hiddenColumns)
      ? legacy.hiddenColumns.filter((id): id is string => typeof id === 'string')
      : [],
  );
  const resolved = resolveLayout(declared, null);
  for (const id of hidden) resolved.visibility[id] = false;
  const density = TABLE_DENSITIES.includes(legacy.density as TableDensity)
    ? (legacy.density as TableDensity)
    : 'default';
  return {
    v: 1,
    columns: toLayoutColumns(declared, resolved),
    density,
    pageSize: typeof legacy.pageSize === 'number' ? legacy.pageSize : tableDefaultPageSize,
    autoRefreshMs: typeof legacy.autoRefreshMs === 'number' ? legacy.autoRefreshMs : 0,
  };
}

function runImport(
  tableId: string,
  declared: DeclaredColumn[],
  data: TableViewData,
  queryClient: QueryClient,
  includeLayout: boolean,
) {
  const prefsKey = `mcharbor-table-${tableId}`;
  const presetsKey = `mcharbor-${tableId}-filters`;

  if (data.layout || data.filters.length > 0) {
    removeKeys(includeLayout ? [prefsKey, presetsKey] : [presetsKey]);
    return;
  }

  const legacy = includeLayout ? readLegacy(prefsKey) : null;
  const presets = loadSearchPresets<Record<string, unknown>>(presetsKey);
  if (!legacy && presets.length === 0) return;

  const body = {
    tables: [
      {
        tableId,
        layout: legacy ? legacyLayout(declared, legacy) : undefined,
        filters: presets.map((preset) => ({
          name: preset.label.trim().slice(0, MAX_PRESET_NAME_LENGTH),
          state: preset.state,
          isDefault: false,
        })),
      },
    ],
  };
  void api.post<{ imported: number }>('/table-views/import', body).then((res) => {
    if (!res.success) return;
    removeKeys(includeLayout ? [prefsKey, presetsKey] : [presetsKey]);
    void queryClient.invalidateQueries({ queryKey: tableViewKey(tableId) });
  });
}

export function useTableViewImport(tableId: string, declared: DeclaredColumn[], query: UseQueryResult<TableViewData>) {
  const queryClient = useQueryClient();
  const attempted = useRef<string | null>(null);

  useEffect(() => {
    if (!query.isSuccess || attempted.current === tableId) return;
    attempted.current = tableId;
    runImport(tableId, declared, query.data, queryClient, true);
  }, [declared, query.data, query.isSuccess, queryClient, tableId]);
}

export function useTableFiltersImport(tableId: string, query: UseQueryResult<TableViewData>) {
  const queryClient = useQueryClient();
  const attempted = useRef<string | null>(null);

  useEffect(() => {
    if (!query.isSuccess || attempted.current === tableId) return;
    attempted.current = tableId;
    runImport(tableId, [], query.data, queryClient, false);
  }, [query.data, query.isSuccess, queryClient, tableId]);
}
