// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Icon as TablerIcon } from '@tabler/icons-react';

export type WidgetTypeId =
  | 'containers'
  | 'images'
  | 'volumes'
  | 'networks'
  | 'cpu-cores'
  | 'total-memory'
  | 'docker-version'
  | 'disk-usage'
  | 'cpu-chart'
  | 'memory-chart'
  | 'network-io-chart'
  | 'disk-io-chart'
  | 'activity-feed'
  | 'lifecycle-feed'
  | 'container-list'
  | 'stack-status'
  | 'resource-donut'
  | 'top-cpu-consumers'
  | 'top-memory-consumers'
  | 'container-states'
  | 'workload-heatmap'
  | 'restart-tracker'
  | 'port-map'
  | 'container-age'
  | 'storage-breakdown'
  | 'capacity-headroom'
  | 'uptime'
  | 'workflow-runs'
  | 'quick-actions'
  | 'alert-summary'
  | 'vulnerability-summary'
  | 'fleet-health'
  | 'needs-attention'
  | 'drift-status'
  | 'instance-backup-status'
  | 'update-queue'
  | 'snapshot-status'
  // Plugin-supplied widget ids are namespaced as `plugin:<slug>:<key>`;
  // they must satisfy the `WidgetTypeId` constraint even though the
  // catalogue can't enumerate them. The string variant lets the
  // bridge accept any plugin-supplied id at the type level.
  | (string & {});

export type WidgetCategory = 'resources' | 'host' | 'metrics' | 'monitoring' | 'operations' | 'plugin';

export type WidgetProps = { colSpan: number; typeId: WidgetTypeId };

export type WidgetDefinitionMeta = {
  id: WidgetTypeId;
  labelKey: string;
  descriptionKey: string;
  category: WidgetCategory;
  icon: TablerIcon;
  defaultSize: { w: number; h: number };
  minSize: { w: number; h: number };
};
