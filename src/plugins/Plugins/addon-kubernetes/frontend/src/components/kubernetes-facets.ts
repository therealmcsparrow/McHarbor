// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// State facets for the resource tables, and the one place a Kubernetes state
// string is turned into a colour.
//
// The containers list answers "is anything wrong?" in one click, through
// `ContainerStateFilterChips` — a band of chips with live counts that composes
// with the text query rather than replacing it. Every Kubernetes table used to
// answer the same question with a regex, or with prior knowledge of the names.
//
// The chips here are derived rather than written out: a tab names the column
// that carries its state (`status` for pods, `type` for events, `kind` for the
// merged tabs), and the facet list is the distinct values actually present,
// counted. That keeps a chip band from claiming a state the cluster is not
// currently reporting, and means a new resource kind needs no new chip code.
//
// `badgeVariant` and the chip tone come from the same `stateTone` map, so a
// pod that is `warning` amber in its row is `warning` amber in its chip and in
// its detail hero.

import type { FilterFacet } from '@resources/components/FilterChips';
import type { StatusTone } from '@resources/theme/status-palette';
import { list } from '../format';
import type { Row } from '../types';
import type { TableId } from './KubernetesTables';

/** The pseudo-facet that clears the filter. Never a real cluster value. */
export const ALL_FACET = '__all__';

/**
 * Which column carries a row's state, per tab.
 *
 * A tab absent from this table renders no chip band — `deployments` has no
 * state column at all (its health is the `ready` fraction, which is data, not
 * a state), and a chip per distinct `2/3` would be noise.
 */
const STATE_COLUMN: Partial<Record<TableId, string>> = {
  pods: 'status',
  namespaces: 'status',
  nodes: 'status',
  storage: 'status',
  capabilities: 'status',
  events: 'type',
  services: 'type',
  config: 'kind',
  workloads: 'kind',
  networking: 'kind',
};

const SUCCESS = new Set(['Running', 'Active', 'Ready', 'True', 'Normal', 'Bound', 'Available', 'Succeeded', 'Healthy']);
const WARNING = new Set([
  'Pending',
  'Warning',
  'False',
  'NotReady',
  'Progressing',
  'Terminating',
  'ContainerCreating',
  'Released',
  'Degraded',
]);
const DANGER = new Set([
  'Failed',
  'Error',
  'CrashLoopBackOff',
  'ImagePullBackOff',
  'ErrImagePull',
  'Evicted',
  'OOMKilled',
  'Lost',
  'Unhealthy',
]);

/** One mapping from a cluster state string to a semantic tone. */
export function stateTone(value: string): StatusTone {
  if (SUCCESS.has(value)) return 'success';
  if (WARNING.has(value)) return 'warning';
  if (DANGER.has(value)) return 'danger';
  return 'neutral';
}

/** The `Badge` variant for a state string, expressed through `stateTone`. */
export function badgeVariant(value: string) {
  switch (stateTone(value)) {
    case 'success':
      return 'success' as const;
    case 'warning':
      return 'warning' as const;
    case 'danger':
      return 'destructive' as const;
    default:
      return 'secondary' as const;
  }
}

/** The state a row reports on this tab, or `null` when the tab has none. */
export function facetValueOf(row: Row, tab: TableId): string | null {
  const column = STATE_COLUMN[tab];
  if (!column) return null;
  const raw = row[column];
  if (raw === null || raw === undefined || raw === '') return null;
  return list(raw);
}

/** Does this row belong to the selected chip? */
export function matchesFacet(row: Row, tab: TableId, facet: string): boolean {
  if (facet === ALL_FACET) return true;
  return facetValueOf(row, tab) === facet;
}

/**
 * The chip band for a tab, or `undefined` when the tab has no state column or
 * only ever reports one value — a single chip beside "All" is a control with
 * nothing to choose.
 *
 * Labels are the cluster's own strings (`Running`, `ConfigMap`, `Warning`):
 * they are Kubernetes API values, not UI copy, and translating them would make
 * them stop matching what `kubectl` prints.
 */
export function buildFacets(rows: Row[], tab: TableId, allLabel: string): FilterFacet<string>[] | undefined {
  if (!STATE_COLUMN[tab]) return undefined;

  const counts = new Map<string, number>();
  for (const row of rows) {
    const value = facetValueOf(row, tab);
    if (value) counts.set(value, (counts.get(value) ?? 0) + 1);
  }
  if (counts.size < 2) return undefined;

  const values = [...counts.keys()].sort((a, b) => a.localeCompare(b));
  return [
    { id: ALL_FACET, label: allLabel, count: rows.length },
    ...values.map((value) => ({
      id: value,
      label: value,
      count: counts.get(value) ?? 0,
      tone: stateTone(value),
    })),
  ];
}
