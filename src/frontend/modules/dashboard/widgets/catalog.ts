// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Icon as TablerIcon } from '@tabler/icons-react';
import {
  IconBoxMultiple,
  IconPhoto,
  IconDeviceFloppy,
  IconNetwork,
  IconCpu,
  IconCpu2,
  IconServer,
  IconDatabase,
  IconActivity,
  IconArrowBarDown,
  IconChartArea,
  IconTimeline,
  IconList,
  IconStack2,
  IconChartDonut,
  IconFlame,
  IconDeviceDesktopAnalytics,
  IconChartPie,
  IconRefresh,
  IconPlug,
  IconClock,
  IconDevices,
  IconPlayerPlay,
  IconBolt,
  IconAlertTriangle,
  IconShieldCheck,
  IconCube,
  IconRocket,
} from '@tabler/icons-react';

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
  | 'container-list'
  | 'stack-status'
  | 'resource-donut'
  | 'top-cpu-consumers'
  | 'top-memory-consumers'
  | 'container-states'
  | 'restart-tracker'
  | 'port-map'
  | 'container-age'
  | 'storage-breakdown'
  | 'uptime'
  | 'workflow-runs'
  | 'quick-actions'
  | 'alert-summary'
  | 'vulnerability-summary'
  | 'pod-status'
  | 'deployment-status';

export type WidgetCategory = 'resources' | 'host' | 'metrics' | 'monitoring' | 'operations' | 'kubernetes';

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

export const WIDGET_CATEGORIES: { id: WidgetCategory; labelKey: string }[] = [
  { id: 'resources', labelKey: 'categories.resources' },
  { id: 'host', labelKey: 'categories.host' },
  { id: 'metrics', labelKey: 'categories.metrics' },
  { id: 'monitoring', labelKey: 'categories.monitoring' },
  { id: 'operations', labelKey: 'categories.operations' },
  { id: 'kubernetes', labelKey: 'categories.kubernetes' },
];

export const BUILTIN_WIDGET_META: Record<WidgetTypeId, WidgetDefinitionMeta> = {
  containers: {
    id: 'containers',
    labelKey: 'widgets.containers.label',
    descriptionKey: 'widgets.containers.description',
    category: 'resources',
    icon: IconBoxMultiple,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  images: {
    id: 'images',
    labelKey: 'widgets.images.label',
    descriptionKey: 'widgets.images.description',
    category: 'resources',
    icon: IconPhoto,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  volumes: {
    id: 'volumes',
    labelKey: 'widgets.volumes.label',
    descriptionKey: 'widgets.volumes.description',
    category: 'resources',
    icon: IconDeviceFloppy,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  networks: {
    id: 'networks',
    labelKey: 'widgets.networks.label',
    descriptionKey: 'widgets.networks.description',
    category: 'resources',
    icon: IconNetwork,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'cpu-cores': {
    id: 'cpu-cores',
    labelKey: 'widgets.cpuCores.label',
    descriptionKey: 'widgets.cpuCores.description',
    category: 'host',
    icon: IconCpu,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'total-memory': {
    id: 'total-memory',
    labelKey: 'widgets.totalMemory.label',
    descriptionKey: 'widgets.totalMemory.description',
    category: 'host',
    icon: IconCpu2,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'docker-version': {
    id: 'docker-version',
    labelKey: 'widgets.dockerVersion.label',
    descriptionKey: 'widgets.dockerVersion.description',
    category: 'host',
    icon: IconServer,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'disk-usage': {
    id: 'disk-usage',
    labelKey: 'widgets.diskUsage.label',
    descriptionKey: 'widgets.diskUsage.description',
    category: 'host',
    icon: IconDatabase,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'cpu-chart': {
    id: 'cpu-chart',
    labelKey: 'widgets.cpuUsage.label',
    descriptionKey: 'widgets.cpuUsage.description',
    category: 'metrics',
    icon: IconActivity,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'memory-chart': {
    id: 'memory-chart',
    labelKey: 'widgets.memoryUsage.label',
    descriptionKey: 'widgets.memoryUsage.description',
    category: 'metrics',
    icon: IconChartArea,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'network-io-chart': {
    id: 'network-io-chart',
    labelKey: 'widgets.networkIO.label',
    descriptionKey: 'widgets.networkIO.description',
    category: 'metrics',
    icon: IconArrowBarDown,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'disk-io-chart': {
    id: 'disk-io-chart',
    labelKey: 'widgets.diskIO.label',
    descriptionKey: 'widgets.diskIO.description',
    category: 'metrics',
    icon: IconTimeline,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'activity-feed': {
    id: 'activity-feed',
    labelKey: 'widgets.activityFeed.label',
    descriptionKey: 'widgets.activityFeed.description',
    category: 'monitoring',
    icon: IconList,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'container-list': {
    id: 'container-list',
    labelKey: 'widgets.containerList.label',
    descriptionKey: 'widgets.containerList.description',
    category: 'monitoring',
    icon: IconBoxMultiple,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'stack-status': {
    id: 'stack-status',
    labelKey: 'widgets.stackStatus.label',
    descriptionKey: 'widgets.stackStatus.description',
    category: 'monitoring',
    icon: IconStack2,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'resource-donut': {
    id: 'resource-donut',
    labelKey: 'widgets.resourceDonut.label',
    descriptionKey: 'widgets.resourceDonut.description',
    category: 'monitoring',
    icon: IconChartDonut,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'top-cpu-consumers': {
    id: 'top-cpu-consumers',
    labelKey: 'widgets.topCpuConsumers.label',
    descriptionKey: 'widgets.topCpuConsumers.description',
    category: 'resources',
    icon: IconFlame,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'top-memory-consumers': {
    id: 'top-memory-consumers',
    labelKey: 'widgets.topMemoryConsumers.label',
    descriptionKey: 'widgets.topMemoryConsumers.description',
    category: 'resources',
    icon: IconDeviceDesktopAnalytics,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'container-states': {
    id: 'container-states',
    labelKey: 'widgets.containerStates.label',
    descriptionKey: 'widgets.containerStates.description',
    category: 'resources',
    icon: IconChartPie,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'restart-tracker': {
    id: 'restart-tracker',
    labelKey: 'widgets.restartTracker.label',
    descriptionKey: 'widgets.restartTracker.description',
    category: 'monitoring',
    icon: IconRefresh,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'port-map': {
    id: 'port-map',
    labelKey: 'widgets.portMap.label',
    descriptionKey: 'widgets.portMap.description',
    category: 'resources',
    icon: IconPlug,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'container-age': {
    id: 'container-age',
    labelKey: 'widgets.containerAge.label',
    descriptionKey: 'widgets.containerAge.description',
    category: 'resources',
    icon: IconClock,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'storage-breakdown': {
    id: 'storage-breakdown',
    labelKey: 'widgets.storageBreakdown.label',
    descriptionKey: 'widgets.storageBreakdown.description',
    category: 'resources',
    icon: IconDevices,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  uptime: {
    id: 'uptime',
    labelKey: 'widgets.uptime.label',
    descriptionKey: 'widgets.uptime.description',
    category: 'host',
    icon: IconClock,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'workflow-runs': {
    id: 'workflow-runs',
    labelKey: 'widgets.workflowRuns.label',
    descriptionKey: 'widgets.workflowRuns.description',
    category: 'operations',
    icon: IconPlayerPlay,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'quick-actions': {
    id: 'quick-actions',
    labelKey: 'widgets.quickActions.label',
    descriptionKey: 'widgets.quickActions.description',
    category: 'operations',
    icon: IconBolt,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'alert-summary': {
    id: 'alert-summary',
    labelKey: 'widgets.alertSummary.label',
    descriptionKey: 'widgets.alertSummary.description',
    category: 'monitoring',
    icon: IconAlertTriangle,
    defaultSize: { w: 4, h: 2 },
    minSize: { w: 3, h: 2 },
  },
  'vulnerability-summary': {
    id: 'vulnerability-summary',
    labelKey: 'widgets.vulnerabilitySummary.label',
    descriptionKey: 'widgets.vulnerabilitySummary.description',
    category: 'monitoring',
    icon: IconShieldCheck,
    defaultSize: { w: 4, h: 2 },
    minSize: { w: 3, h: 2 },
  },
  'pod-status': {
    id: 'pod-status',
    labelKey: 'widgets.podStatus.label',
    descriptionKey: 'widgets.podStatus.description',
    category: 'kubernetes',
    icon: IconCube,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'deployment-status': {
    id: 'deployment-status',
    labelKey: 'widgets.deploymentStatus.label',
    descriptionKey: 'widgets.deploymentStatus.description',
    category: 'kubernetes',
    icon: IconRocket,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
};

export const BUILTIN_WIDGET_DEFINITIONS = Object.values(BUILTIN_WIDGET_META);

export function getWidgetMeta(typeId: string): WidgetDefinitionMeta | undefined {
  return BUILTIN_WIDGET_META[typeId as WidgetTypeId];
}
