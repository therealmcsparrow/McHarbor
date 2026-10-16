// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What is wrong with this cluster, in the order a reader should care.
//
// The container overview opens with `useContainerInsights` — a derived list of
// observations ("unhealthy for 4 checks", "OOM killed", "3 critical CVEs"),
// each with a severity and a tab to jump to. That is the part of the container
// overview that does the actual work: the reader is told what to look at rather
// than left to compare eleven counts against each other.
//
// The cluster summary already carries everything needed to answer the same
// question — it ships every pod, node, event and deployment row, not just the
// counts — so this derives the same shape from it. Pure and synchronous: the
// summary is one already-cached query, so there is nothing here to await and
// no hook to write.

import { list } from '../format';
import type { TabId } from '../nav';
import type { KubernetesSummary, Row } from '../types';
import { stateTone } from './kubernetes-facets';

export type InsightSeverity = 'info' | 'warning' | 'error';

export type ClusterInsight = {
  id: string;
  severity: InsightSeverity;
  /** i18n key, resolved by the card against the `kubernetes` namespace. */
  titleKey: string;
  descriptionKey: string;
  values?: Record<string, string | number>;
  /** Where the arrow takes the reader. */
  target: TabId;
};

/** A deployment is short of its target when `ready` reads `a/b` with a ≠ b. */
function rolloutIncomplete(row: Row): boolean {
  const [current, desired] = list(row.ready).split('/');
  return Boolean(desired) && current !== desired;
}

export function clusterInsights(summary: KubernetesSummary): ClusterInsight[] {
  const insights: ClusterInsight[] = [];

  const failedPods = summary.pods.filter((pod) => stateTone(list(pod.status)) === 'danger');
  const pendingPods = summary.pods.filter((pod) => stateTone(list(pod.status)) === 'warning');
  // A node reports `Ready` or it does not. Anything else — `NotReady`,
  // `SchedulingDisabled`, `Unknown` — is a node the scheduler cannot fully
  // use, and the reader wants all of them in one number.
  const unreadyNodes = summary.nodes.filter((node) => list(node.status) !== 'Ready');
  const warningEvents = summary.events.filter((evt) => list(evt.type) === 'Warning');
  const stalledRollouts = summary.deployments.filter(rolloutIncomplete);
  const degradedProbes = summary.capabilities.filter((probe) => stateTone(list(probe.status)) !== 'success');

  if (failedPods.length > 0) {
    insights.push({
      id: 'failed-pods',
      severity: 'error',
      titleKey: 'insights.failedPodsTitle',
      descriptionKey: 'insights.failedPodsDescription',
      values: { count: failedPods.length, name: list(failedPods[0]?.name) },
      target: 'pods',
    });
  }

  if (unreadyNodes.length > 0) {
    insights.push({
      id: 'unready-nodes',
      severity: 'error',
      titleKey: 'insights.unreadyNodesTitle',
      descriptionKey: 'insights.unreadyNodesDescription',
      values: { count: unreadyNodes.length, name: list(unreadyNodes[0]?.name) },
      target: 'nodes',
    });
  }

  if (stalledRollouts.length > 0) {
    insights.push({
      id: 'stalled-rollouts',
      severity: 'warning',
      titleKey: 'insights.rolloutTitle',
      descriptionKey: 'insights.rolloutDescription',
      values: { count: stalledRollouts.length, name: list(stalledRollouts[0]?.name) },
      target: 'deployments',
    });
  }

  if (pendingPods.length > 0) {
    insights.push({
      id: 'pending-pods',
      severity: 'warning',
      titleKey: 'insights.pendingPodsTitle',
      descriptionKey: 'insights.pendingPodsDescription',
      values: { count: pendingPods.length },
      target: 'pods',
    });
  }

  if (warningEvents.length > 0) {
    insights.push({
      id: 'warning-events',
      severity: 'warning',
      titleKey: 'insights.warningEventsTitle',
      descriptionKey: 'insights.warningEventsDescription',
      values: { count: warningEvents.length, reason: list(warningEvents[0]?.reason) },
      target: 'events',
    });
  }

  if (degradedProbes.length > 0) {
    insights.push({
      id: 'degraded-probes',
      severity: 'info',
      titleKey: 'insights.probesTitle',
      descriptionKey: 'insights.probesDescription',
      values: { count: degradedProbes.length },
      target: 'capabilities',
    });
  }

  return insights;
}

/**
 * The three or four things worth doing next, chosen from what the cluster is
 * currently reporting — the cluster's answer to the container overview's
 * start / view logs / edit config band.
 */
export type SuggestedActionId = 'inspectPods' | 'reviewEvents' | 'viewNodes' | 'viewWorkloads' | 'manageClusters';

export type SuggestedAction = {
  id: SuggestedActionId;
  /** Plugin-relative destination. */
  to: string;
  /** The one the reader most likely wants, rendered filled rather than outline. */
  primary?: boolean;
  /** Set only where the destination is gated on more than `kubernetes.view`. */
  permission?: string;
};

export function suggestedActions(summary: KubernetesSummary, clustersPermission: string): SuggestedAction[] {
  const broken = summary.pods.some((pod) => stateTone(list(pod.status)) === 'danger');
  const warned = summary.events.some((evt) => list(evt.type) === 'Warning');

  const clusters: SuggestedAction = {
    id: 'manageClusters',
    to: 'clusters',
    permission: clustersPermission,
  };

  if (broken) {
    return [
      { id: 'inspectPods', to: 'pods', primary: true },
      { id: 'reviewEvents', to: 'events' },
      { id: 'viewNodes', to: 'nodes' },
    ];
  }

  if (warned) {
    return [{ id: 'reviewEvents', to: 'events', primary: true }, { id: 'viewWorkloads', to: 'pods' }, clusters];
  }

  return [{ id: 'viewWorkloads', to: 'pods' }, { id: 'viewNodes', to: 'nodes' }, clusters];
}
