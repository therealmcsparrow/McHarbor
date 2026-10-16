// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { TablePageSavedFilters } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { isHostServiceEnabled, isHostServiceRunning, type HostService } from '@resources/hooks/useHostServices';

export type HostServiceFacet = 'all' | 'running' | 'failed' | 'inactive' | 'enabled' | 'disabled';

const FACETS: {
  id: Exclude<HostServiceFacet, 'all'>;
  match: (unit: HostService) => boolean;
}[] = [
  { id: 'running', match: isHostServiceRunning },
  { id: 'failed', match: (unit) => unit.activeState === 'failed' },
  { id: 'inactive', match: (unit) => unit.activeState === 'inactive' },
  { id: 'enabled', match: isHostServiceEnabled },
  { id: 'disabled', match: (unit) => unit.unitFileState === 'disabled' },
];

const FACET_IDS = new Set<string>(['all', ...FACETS.map((facet) => facet.id)]);

function inFacet(unit: HostService, facet: HostServiceFacet): boolean {
  if (facet === 'all') return true;
  return FACETS.find((f) => f.id === facet)?.match(unit) ?? false;
}

export function useHostServicesFilters(units: HostService[]) {
  const { t } = useTranslation('environments');
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<HostServiceFacet>('all');

  const rows = units.filter((unit) => inFacet(unit, facet));

  const items: FilterFacet<string>[] = [
    {
      id: 'all',
      label: t('labels.all', { ns: 'common' }),
      count: units.length,
    },
    ...FACETS.map((f) => ({
      id: f.id,
      label: t(`hostServices.facet.${f.id}`),
      count: units.filter(f.match).length,
    })),
  ];

  const savedFilters: TablePageSavedFilters = {
    state: { query, mode, facet },
    onRestore: (state) => {
      setQuery(typeof state.query === 'string' ? state.query : '');
      setMode(state.mode === 'exact' || state.mode === 'regex' ? state.mode : 'contains');
      setFacet(
        typeof state.facet === 'string' && FACET_IDS.has(state.facet) ? (state.facet as HostServiceFacet) : 'all',
      );
    },
    pristine: !query && mode === 'contains' && facet === 'all',
    defaults: { query: '', mode: 'contains', facet: 'all' },
  };

  return {
    rows,
    search: { value: query, onChange: setQuery, mode, onModeChange: setMode },
    facets: {
      items,
      value: facet,
      onChange: (next: string) => setFacet(next as HostServiceFacet),
    },
    savedFilters,
  };
}
