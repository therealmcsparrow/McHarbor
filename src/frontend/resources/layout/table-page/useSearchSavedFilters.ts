// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import type { TablePageSavedFilters, TableSearchControl } from './types';

export function useSearchSavedFilters(search: TableSearchControl | undefined): TablePageSavedFilters | undefined {
  return useMemo<TablePageSavedFilters | undefined>(
    () =>
      search
        ? {
            state: { query: search.value, mode: search.mode },
            onRestore: (state) => {
              search.onChange(typeof state.query === 'string' ? state.query : '');
              if (state.mode === 'contains' || state.mode === 'exact' || state.mode === 'regex') {
                search.onModeChange(state.mode);
              }
            },
            pristine: search.value === '' && search.mode === 'contains',
            defaults: { query: '', mode: 'contains' },
          }
        : undefined,
    [search],
  );
}
