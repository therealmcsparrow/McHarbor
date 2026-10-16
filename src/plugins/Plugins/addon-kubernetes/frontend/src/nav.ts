// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TableId } from './components/KubernetesTables';

export type TabId = 'overview' | 'resources' | TableId;

export type KubernetesGroupId = 'overview' | 'workloads' | 'network' | 'storage' | 'config' | 'cluster';

export type KubernetesGroup = {
  id: KubernetesGroupId;
  // The tabs this group owns, in strip order. The first one is where the
  // sidemenu item lands.
  tabs: TabId[];
};

// How the cluster is divided in the sidebar.
//
// The split follows the convention the Kubernetes Dashboard, Lens and Rancher
// all share: group resources by the job they do for an application — what runs,
// how it is reached, where it keeps state, what configures it, and what the
// cluster itself is made of — rather than by API group.
//
// Tab ids still name the URL segment: each sidemenu item points at its first
// tab, so `/plugins/addon-kubernetes/pods` and the
// `/plugins/addon-kubernetes/pods/<ns>/<name>` detail routes follow from the same
// table the sidebar reads.
// The cluster root. Named so the lookups below have a total fallback rather
// than an index the compiler cannot prove is there.
export const kubernetesOverviewGroup: KubernetesGroup = {
  id: 'overview',
  tabs: ['overview', 'resources'],
};

export const kubernetesGroups: KubernetesGroup[] = [
  kubernetesOverviewGroup,
  { id: 'workloads', tabs: ['pods', 'deployments', 'workloads'] },
  { id: 'network', tabs: ['services', 'networking'] },
  { id: 'storage', tabs: ['storage'] },
  { id: 'config', tabs: ['config'] },
  { id: 'cluster', tabs: ['nodes', 'namespaces', 'events', 'capabilities'] },
];

export const kubernetesTabs: TabId[] = kubernetesGroups.flatMap((group) => group.tabs);

// `overview` is the cluster root, every other tab is a path segment under it.
// Plugin-relative: `pluginHref` in nav-context adds the /plugins/addon-kubernetes
// prefix, so this table stays the single place tab ids map to URL segments.
export function kubernetesTabPath(tab: TabId): string {
  return tab === 'overview' ? '' : tab;
}

export function kubernetesGroupOf(tab: TabId): KubernetesGroup {
  return kubernetesGroups.find((group) => group.tabs.includes(tab)) ?? kubernetesOverviewGroup;
}

// Where a sidemenu item for this group points.
export function kubernetesGroupPath(group: KubernetesGroup): string {
  return kubernetesTabPath(group.tabs[0] ?? 'overview');
}

// Every path that should light the group's sidemenu item — its own tabs, plus
// (by prefix) the detail routes hanging off them.
export function kubernetesGroupMatchPaths(group: KubernetesGroup): string[] {
  return group.tabs.map(kubernetesTabPath);
}
