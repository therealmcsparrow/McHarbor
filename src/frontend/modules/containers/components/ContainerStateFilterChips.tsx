// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The container-list state facets.
//
// Answers the question people actually arrive with — "is anything wrong?"
// — in one click, where before it took a regex or prior knowledge of the
// names. Counts are live, so an empty "Unhealthy" chip is itself the answer.

import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { FilterChips, type FilterFacet } from '@resources/components/FilterChips';
import { parseHealth } from './container-utils';

export const CONTAINER_STATE_FACETS = ['all', 'running', 'unhealthy', 'paused', 'exited', 'update'] as const;

export type ContainerStateFacet = (typeof CONTAINER_STATE_FACETS)[number];

/**
 * Does this container belong to the facet? Exported so the page filters
 * with exactly the predicate the chip counted with — a chip that says 3 and
 * a list that shows 4 is worse than no chip at all.
 */
export function matchesStateFacet(
  container: ContainerInfo,
  facet: ContainerStateFacet,
  updateAvailableIDs: ReadonlySet<string>,
): boolean {
  switch (facet) {
    case 'all':
      return true;
    case 'running':
      return container.State === 'running';
    case 'unhealthy':
      return parseHealth(container.Status) === 'unhealthy';
    case 'paused':
      return container.State === 'paused';
    case 'exited':
      return container.State === 'exited' || container.State === 'dead';
    case 'update':
      return updateAvailableIDs.has(container.Id);
    default:
      return true;
  }
}

type ContainerStateFilterChipsProps = {
  containers: ContainerInfo[];
  updateAvailableIDs: ReadonlySet<string>;
  value: ContainerStateFacet;
  onChange: (next: ContainerStateFacet) => void;
};

export function ContainerStateFilterChips({
  containers,
  updateAvailableIDs,
  value,
  onChange,
}: ContainerStateFilterChipsProps) {
  const { t } = useTranslation('containers');

  const count = (facet: ContainerStateFacet) =>
    containers.filter((c) => matchesStateFacet(c, facet, updateAvailableIDs)).length;

  const facets: FilterFacet<ContainerStateFacet>[] = [
    { id: 'all', label: t('facets.all'), count: count('all') },
    {
      id: 'running',
      label: t('facets.running'),
      count: count('running'),
      tone: 'success',
    },
    {
      id: 'unhealthy',
      label: t('facets.unhealthy'),
      count: count('unhealthy'),
      tone: 'danger',
    },
    {
      id: 'paused',
      label: t('facets.paused'),
      count: count('paused'),
      tone: 'warning',
    },
    {
      id: 'exited',
      label: t('facets.exited'),
      count: count('exited'),
      tone: 'neutral',
    },
    {
      id: 'update',
      label: t('facets.updateReady'),
      count: count('update'),
      tone: 'info',
    },
  ];

  return <FilterChips facets={facets} value={value} onChange={onChange} label={t('facets.label')} />;
}
