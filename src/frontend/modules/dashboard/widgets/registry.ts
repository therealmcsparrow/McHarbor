// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, type ComponentType } from 'react';
import {
  BUILTIN_WIDGET_DEFINITIONS,
  WIDGET_CATEGORIES,
  type WidgetCategory,
  type WidgetDefinitionMeta,
  type WidgetProps,
  type WidgetTypeId,
} from './catalog';
import { useWidgetRegistryStore } from '../stores/widget-registry';

export type WidgetDefinition = WidgetDefinitionMeta & {
  component: ComponentType<WidgetProps>;
};

const ResourceSummaryWidget = lazy(() => import('@widgets/resource-summary/ResourceSummaryWidget'));
const HostInfoWidget = lazy(() => import('@widgets/host-info/HostInfoWidget'));
const MetricChartWidget = lazy(() => import('@widgets/metric-chart/MetricChartWidget'));
const ActivityFeedWidget = lazy(() => import('@widgets/activity-feed/ActivityFeedWidget'));
const ContainerListWidget = lazy(() => import('@widgets/container-list/ContainerListWidget'));
const StackStatusWidget = lazy(() => import('@widgets/stack-status/StackStatusWidget'));
const ResourceDonutWidget = lazy(() => import('@widgets/resource-donut/ResourceDonutWidget'));
const TopConsumersWidget = lazy(() => import('@widgets/top-consumers/TopConsumersWidget'));
const ContainerStatesWidget = lazy(() => import('@widgets/container-states/ContainerStatesWidget'));
const RestartTrackerWidget = lazy(() => import('@widgets/restart-tracker/RestartTrackerWidget'));
const PortMapWidget = lazy(() => import('@widgets/port-map/PortMapWidget'));
const ContainerAgeWidget = lazy(() => import('@widgets/container-age/ContainerAgeWidget'));
const StorageBreakdownWidget = lazy(() => import('@widgets/storage-breakdown/StorageBreakdownWidget'));
const WorkflowRunsWidget = lazy(() => import('@widgets/workflow-runs/WorkflowRunsWidget'));
const QuickActionsWidget = lazy(() => import('@widgets/quick-actions/QuickActionsWidget'));
const AlertSummaryWidget = lazy(() => import('@widgets/alert-summary/AlertSummaryWidget'));
const VulnerabilitySummaryWidget = lazy(() => import('@widgets/vulnerability-summary/VulnerabilitySummaryWidget'));
const PodStatusWidget = lazy(() => import('@widgets/pod-status/PodStatusWidget'));
const DeploymentStatusWidget = lazy(() => import('@widgets/deployment-status/DeploymentStatusWidget'));

function resolveWidgetComponent(typeId: WidgetTypeId): ComponentType<WidgetProps> {
  switch (typeId) {
    case 'containers':
    case 'images':
    case 'volumes':
    case 'networks':
      return ResourceSummaryWidget;
    case 'cpu-cores':
    case 'total-memory':
    case 'docker-version':
    case 'disk-usage':
    case 'uptime':
      return HostInfoWidget;
    case 'cpu-chart':
    case 'memory-chart':
    case 'network-io-chart':
    case 'disk-io-chart':
      return MetricChartWidget;
    case 'activity-feed':
      return ActivityFeedWidget;
    case 'container-list':
      return ContainerListWidget;
    case 'stack-status':
      return StackStatusWidget;
    case 'resource-donut':
      return ResourceDonutWidget;
    case 'top-cpu-consumers':
    case 'top-memory-consumers':
      return TopConsumersWidget;
    case 'container-states':
      return ContainerStatesWidget;
    case 'restart-tracker':
      return RestartTrackerWidget;
    case 'port-map':
      return PortMapWidget;
    case 'container-age':
      return ContainerAgeWidget;
    case 'storage-breakdown':
      return StorageBreakdownWidget;
    case 'workflow-runs':
      return WorkflowRunsWidget;
    case 'quick-actions':
      return QuickActionsWidget;
    case 'alert-summary':
      return AlertSummaryWidget;
    case 'vulnerability-summary':
      return VulnerabilitySummaryWidget;
    case 'pod-status':
      return PodStatusWidget;
    case 'deployment-status':
      return DeploymentStatusWidget;
  }
}

export const WIDGET_REGISTRY: Record<WidgetTypeId, WidgetDefinition> = Object.fromEntries(
  BUILTIN_WIDGET_DEFINITIONS.map((definition) => [
    definition.id,
    {
      ...definition,
      component: resolveWidgetComponent(definition.id),
    },
  ]),
) as Record<WidgetTypeId, WidgetDefinition>;

useWidgetRegistryStore.getState().setDefinitions(Object.values(WIDGET_REGISTRY));

export const useWidgetDefinitions = () => useWidgetRegistryStore((s) => s.definitions);

export const useWidgetDefinitionMap = () => useWidgetRegistryStore((s) => s.definitionMap);

export function getWidgetDefinitionMap(): Record<string, WidgetDefinition> {
  return useWidgetRegistryStore.getState().definitionMap;
}

export {
  BUILTIN_WIDGET_DEFINITIONS,
  WIDGET_CATEGORIES,
  type WidgetCategory,
  type WidgetProps,
  type WidgetTypeId,
};
