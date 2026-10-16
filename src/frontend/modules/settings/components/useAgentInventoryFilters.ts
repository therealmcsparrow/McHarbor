// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { ActiveFilterChip, TablePageSavedFilters } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { agentPlatform, agentState, type AgentRow, type AgentState } from '@resources/hooks/useAgentInventory';

export type AgentStatusFacet = 'all' | 'current' | 'outdated' | 'excluded' | 'tooOld' | 'inProgress' | 'failed';
export type AgentUpdateFilter = '' | 'yes' | 'no';

const FACETS: { id: Exclude<AgentStatusFacet, 'all'>; key: string; states: AgentState[] }[] = [
  { id: 'current', key: 'about.agents.current', states: ['current'] },
  { id: 'outdated', key: 'about.agents.outdated', states: ['outdated'] },
  { id: 'excluded', key: 'about.agents.excluded', states: ['excluded'] },
  { id: 'tooOld', key: 'about.agents.tooOld', states: ['tooOld'] },
  { id: 'inProgress', key: 'about.agents.facet.inProgress', states: ['pending', 'updating', 'started'] },
  { id: 'failed', key: 'about.agents.progress.failed', states: ['failed'] },
];

const FACET_IDS = new Set<string>(['all', ...FACETS.map((facet) => facet.id)]);

function distinct(values: (string | undefined)[]): string[] {
  return [...new Set(values.filter((value): value is string => Boolean(value)))].sort();
}

export function useAgentInventoryFilters(agents: AgentRow[], excluded: ReadonlySet<string>) {
  const { t } = useTranslation('settings');
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [statusFacet, setStatus] = useState<AgentStatusFacet>('all');
  const [update, setUpdate] = useState<AgentUpdateFilter>('');
  const [platform, setPlatform] = useState('');
  const [version, setVersion] = useState('');

  const stateOf = useMemo(() => {
    const map = new Map<string, AgentState>();
    for (const agent of agents) map.set(agent.envId, agentState(agent, excluded.has(agent.envId)));
    return map;
  }, [agents, excluded]);

  const inFacet = (agent: AgentRow, facet: AgentStatusFacet) =>
    facet === 'all' ||
    (FACETS.find((f) => f.id === facet)?.states.includes(stateOf.get(agent.envId) ?? 'current') ?? false);

  const rows = agents.filter(
    (agent) =>
      inFacet(agent, statusFacet) &&
      (update === '' || agent.outdated === (update === 'yes')) &&
      (platform === '' || agentPlatform(agent) === platform) &&
      (version === '' || agent.version === version),
  );

  const facets: FilterFacet<string>[] = [
    { id: 'all', label: t('labels.all', { ns: 'common' }), count: agents.length },
    ...FACETS.map((facet) => ({
      id: facet.id,
      label: t(facet.key),
      count: agents.filter((agent) => inFacet(agent, facet.id)).length,
    })),
  ];

  const platforms = useMemo(() => distinct(agents.map(agentPlatform)), [agents]);
  const versions = useMemo(() => distinct(agents.map((agent) => agent.version)), [agents]);

  const reset = () => {
    setStatus('all');
    setUpdate('');
    setPlatform('');
    setVersion('');
  };

  const chips: ActiveFilterChip[] = [
    ...(update
      ? [
          {
            id: 'update',
            label: `${t('about.agents.updateAvailable')}: ${t(update === 'yes' ? 'labels.yes' : 'labels.no', { ns: 'common' })}`,
            onRemove: () => setUpdate(''),
          },
        ]
      : []),
    ...(platform
      ? [{ id: 'platform', label: `${t('about.agents.platform')}: ${platform}`, onRemove: () => setPlatform('') }]
      : []),
    ...(version
      ? [{ id: 'version', label: `${t('about.agents.version')}: ${version}`, onRemove: () => setVersion('') }]
      : []),
  ];

  const savedFilters: TablePageSavedFilters = {
    state: { query, mode, status: statusFacet, update, platform, version },
    onRestore: (state) => {
      setQuery(typeof state.query === 'string' ? state.query : '');
      setMode(state.mode === 'exact' || state.mode === 'regex' ? state.mode : 'contains');
      setStatus(
        typeof state.status === 'string' && FACET_IDS.has(state.status) ? (state.status as AgentStatusFacet) : 'all',
      );
      setUpdate(state.update === 'yes' || state.update === 'no' ? state.update : '');
      setPlatform(typeof state.platform === 'string' ? state.platform : '');
      setVersion(typeof state.version === 'string' ? state.version : '');
    },
    pristine: !query && mode === 'contains' && statusFacet === 'all' && !update && !platform && !version,
    defaults: { query: '', mode: 'contains', status: 'all', update: '', platform: '', version: '' },
  };

  return {
    rows,
    search: { value: query, onChange: setQuery, mode, onModeChange: setMode },
    facets: { items: facets, value: statusFacet, onChange: (next: string) => setStatus(next as AgentStatusFacet) },
    selects: { update, setUpdate, platform, setPlatform, platforms, version, setVersion, versions },
    chips: { items: chips, onClear: reset },
    savedFilters,
  };
}
