// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { ActiveFilterChip, TablePageSavedFilters } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import type { ClusterInstance, HARole } from '../hooks/useCluster';

export type NodeHealth = 'up' | 'degraded' | 'down';
export type NodeFacet = 'all' | NodeHealth | 'drift';
export type RoleFilter = '' | HARole;

const HEALTH: NodeHealth[] = ['up', 'degraded', 'down'];
const FACET_IDS = new Set<string>(['all', ...HEALTH, 'drift']);
const ROLES = new Set<string>(['active', 'standby', 'peer']);

export function nodeHealth(node: ClusterInstance): NodeHealth {
  if (!node.alive) return 'down';
  return node.state === 'running' ? 'up' : 'degraded';
}

export function releaseOf(node: ClusterInstance): string {
  return `${node.version} · ${node.build}`;
}

export function driftReference(instances: ClusterInstance[], nodeId: string): string | null {
  const own = instances.find((node) => node.self) ?? instances.find((node) => node.id === nodeId);
  if (own) return releaseOf(own);
  const counts = new Map<string, number>();
  for (const node of instances) counts.set(releaseOf(node), (counts.get(releaseOf(node)) ?? 0) + 1);
  return [...counts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? null;
}

export function useClusterInstanceFilters(instances: ClusterInstance[], nodeId: string) {
  const { t } = useTranslation('enterprise');
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<NodeFacet>('all');
  const [role, setRole] = useState<RoleFilter>('');

  const reference = useMemo(() => driftReference(instances, nodeId), [instances, nodeId]);
  const drifted = (node: ClusterInstance) => reference !== null && releaseOf(node) !== reference;

  const inFacet = (node: ClusterInstance, value: NodeFacet) =>
    value === 'all' || (value === 'drift' ? drifted(node) : nodeHealth(node) === value);

  const rows = instances.filter((node) => inFacet(node, facet) && (role === '' || node.haRole === role));

  const facets: FilterFacet<string>[] = [
    { id: 'all', label: t('labels.all', { ns: 'common' }), count: instances.length },
    ...HEALTH.map((health) => ({
      id: health,
      label: t(`ha.health.${health}`),
      count: instances.filter((node) => nodeHealth(node) === health).length,
    })),
    { id: 'drift', label: t('ha.facet.drift'), count: instances.filter(drifted).length },
  ];

  const reset = () => {
    setFacet('all');
    setRole('');
  };

  const chips: ActiveFilterChip[] = role
    ? [{ id: 'role', label: `${t('ha.columns.role')}: ${t(`ha.role.${role}`)}`, onRemove: () => setRole('') }]
    : [];

  const savedFilters: TablePageSavedFilters = {
    state: { query, mode, facet, role },
    onRestore: (state) => {
      setQuery(typeof state.query === 'string' ? state.query : '');
      setMode(state.mode === 'exact' || state.mode === 'regex' ? state.mode : 'contains');
      setFacet(typeof state.facet === 'string' && FACET_IDS.has(state.facet) ? (state.facet as NodeFacet) : 'all');
      setRole(typeof state.role === 'string' && ROLES.has(state.role) ? (state.role as HARole) : '');
    },
    pristine: !query && mode === 'contains' && facet === 'all' && !role,
    defaults: { query: '', mode: 'contains', facet: 'all', role: '' },
  };

  return {
    rows,
    drifted,
    search: { value: query, onChange: setQuery, mode, onModeChange: setMode },
    facets: { items: facets, value: facet, onChange: (next: string) => setFacet(next as NodeFacet) },
    role: { value: role, onChange: setRole },
    chips: { items: chips, onClear: reset },
    savedFilters,
  };
}
