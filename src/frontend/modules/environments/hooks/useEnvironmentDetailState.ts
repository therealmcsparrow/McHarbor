// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { normalizeTimezone } from '../timezones';
import type { EnvironmentInfo } from './useEnvironments';

export type EnvironmentDetailTab =
  | 'overview'
  | 'activity'
  | 'automation'
  | 'retention'
  | 'host'
  | 'docker'
  | 'processes'
  | 'terminal'
  | 'logs';

const ALL_DAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const;
type WeekDay = (typeof ALL_DAYS)[number];
export type AutoUpdateDay = WeekDay;

function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) {
    return 80;
  }

  return Math.min(100, Math.max(1, Math.round(value)));
}

function clampDayCount(value: number): number {
  if (!Number.isFinite(value)) {
    return 7;
  }

  return Math.min(3650, Math.max(1, Math.round(value)));
}

function clampHours(value: number): number {
  if (!Number.isFinite(value)) {
    return 24;
  }

  return Math.min(8760, Math.max(0, Math.round(value)));
}

function parseSelectedDays(value: string): Set<AutoUpdateDay> {
  const next = new Set<AutoUpdateDay>();
  for (const raw of value.split(',')) {
    const day = raw.trim().toLowerCase() as AutoUpdateDay;
    if ((ALL_DAYS as readonly string[]).includes(day)) {
      next.add(day);
    }
  }
  return next;
}

function serializeSelectedDays(selected: Set<AutoUpdateDay>): string {
  return ALL_DAYS.filter((day) => selected.has(day as AutoUpdateDay)).join(',');
}

export function useEnvironmentDetailState(env?: EnvironmentInfo) {
  const [tokenDialogOpen, setTokenDialogOpen] = useState(false);
  const [regeneratedToken, setRegeneratedToken] = useState('');
  const [activeTab, setActiveTab] = useState<EnvironmentDetailTab>('overview');
  const [trackContainerEventsEnabled, setTrackContainerEventsEnabled] = useState(true);
  const [collectContainerMetricsEnabled, setCollectContainerMetricsEnabled] = useState(true);
  const [highlightContainerChangesEnabled, setHighlightContainerChangesEnabled] = useState(true);
  const [dockerDiskUsageNotificationsEnabled, setDockerDiskUsageNotificationsEnabled] = useState(true);
  const [dockerDiskUsageThresholdPercent, setDockerDiskUsageThresholdPercent] = useState('80');
  const [dockerDiskUsageUseGlobalDefault, setDockerDiskUsageUseGlobalDefault] = useState(true);
  const [scheduledUpdateCheckEnabled, setScheduledUpdateCheckEnabled] = useState(false);
  const [automaticImagePruningEnabled, setAutomaticImagePruningEnabled] = useState(false);
  const [imagePruneDanglingOnly, setImagePruneDanglingOnly] = useState(false);
  const [timezone, setTimezone] = useState('UTC');
  const [logRetentionDays, setLogRetentionDays] = useState('7');
  const [containerPruneEnabled, setContainerPruneEnabled] = useState(false);
  const [containerPruneStoppedDays, setContainerPruneStoppedDays] = useState('7');
  const [autoUpdateEnabled, setAutoUpdateEnabled] = useState(false);
  const [autoUpdateWindowStart, setAutoUpdateWindowStart] = useState('02:00');
  const [autoUpdateWindowEnd, setAutoUpdateWindowEnd] = useState('05:00');
  const [autoUpdateDaysSelected, setAutoUpdateDaysSelected] = useState<Set<AutoUpdateDay>>(
    () => new Set<AutoUpdateDay>(ALL_DAYS),
  );
  const [metricRetentionHours, setMetricRetentionHours] = useState('24');

  useEffect(() => {
    if (!env) {
      return;
    }

    setTrackContainerEventsEnabled(env.trackContainerEventsEnabled);
    setCollectContainerMetricsEnabled(env.collectContainerMetricsEnabled);
    setHighlightContainerChangesEnabled(env.highlightContainerChangesEnabled);
    setDockerDiskUsageNotificationsEnabled(env.dockerDiskUsageNotificationsEnabled);
    setDockerDiskUsageThresholdPercent(String(env.dockerDiskUsageThresholdPercent));
    setDockerDiskUsageUseGlobalDefault(env.dockerDiskUsageUseGlobalDefault);
    setScheduledUpdateCheckEnabled(env.scheduledUpdateCheckEnabled);
    setAutomaticImagePruningEnabled(env.automaticImagePruningEnabled);
    setImagePruneDanglingOnly(env.imagePruneDanglingOnly);
    setTimezone(normalizeTimezone(env.timezone));
    setLogRetentionDays(String(env.logRetentionDays));
    setContainerPruneEnabled(env.containerPruneEnabled);
    setContainerPruneStoppedDays(String(env.containerPruneStoppedDays));
    setAutoUpdateEnabled(env.autoUpdateEnabled);
    setAutoUpdateWindowStart(env.autoUpdateWindowStart || '02:00');
    setAutoUpdateWindowEnd(env.autoUpdateWindowEnd || '05:00');
    setAutoUpdateDaysSelected(parseSelectedDays(env.autoUpdateDays));
    setMetricRetentionHours(String(env.metricRetentionHours));
  }, [env]);

  const normalizedThreshold = clampThreshold(Number.parseInt(dockerDiskUsageThresholdPercent, 10));
  const normalizedLogRetention = clampDayCount(Number.parseInt(logRetentionDays, 10));
  const normalizedContainerPruneDays = clampDayCount(Number.parseInt(containerPruneStoppedDays, 10));
  const normalizedMetricHours = clampHours(Number.parseInt(metricRetentionHours, 10));
  const serializedAutoUpdateDays = serializeSelectedDays(autoUpdateDaysSelected);

  const activityIsDirty = env
    ? trackContainerEventsEnabled !== env.trackContainerEventsEnabled ||
      collectContainerMetricsEnabled !== env.collectContainerMetricsEnabled ||
      highlightContainerChangesEnabled !== env.highlightContainerChangesEnabled ||
      dockerDiskUsageNotificationsEnabled !== env.dockerDiskUsageNotificationsEnabled ||
      normalizedThreshold !== env.dockerDiskUsageThresholdPercent ||
      dockerDiskUsageUseGlobalDefault !== env.dockerDiskUsageUseGlobalDefault
    : false;
  const automationIsDirty = env
    ? scheduledUpdateCheckEnabled !== env.scheduledUpdateCheckEnabled ||
      automaticImagePruningEnabled !== env.automaticImagePruningEnabled ||
      imagePruneDanglingOnly !== env.imagePruneDanglingOnly ||
      timezone !== normalizeTimezone(env.timezone) ||
      autoUpdateEnabled !== env.autoUpdateEnabled ||
      autoUpdateWindowStart !== (env.autoUpdateWindowStart || '02:00') ||
      autoUpdateWindowEnd !== (env.autoUpdateWindowEnd || '05:00') ||
      serializedAutoUpdateDays !== env.autoUpdateDays
    : false;
  const retentionIsDirty = env
    ? normalizedLogRetention !== env.logRetentionDays ||
      containerPruneEnabled !== env.containerPruneEnabled ||
      normalizedContainerPruneDays !== env.containerPruneStoppedDays ||
      normalizedMetricHours !== env.metricRetentionHours
    : false;

  return {
    tokenDialogOpen,
    setTokenDialogOpen,
    regeneratedToken,
    setRegeneratedToken,
    activeTab,
    setActiveTab,
    trackContainerEventsEnabled,
    collectContainerMetricsEnabled,
    highlightContainerChangesEnabled,
    dockerDiskUsageNotificationsEnabled,
    dockerDiskUsageThresholdPercent,
    dockerDiskUsageUseGlobalDefault,
    scheduledUpdateCheckEnabled,
    automaticImagePruningEnabled,
    imagePruneDanglingOnly,
    timezone,
    logRetentionDays,
    containerPruneEnabled,
    containerPruneStoppedDays,
    autoUpdateEnabled,
    autoUpdateWindowStart,
    autoUpdateWindowEnd,
    autoUpdateDaysSelected,
    setAutoUpdateDaysSelected,
    metricRetentionHours,
    normalizedThreshold,
    normalizedLogRetention,
    normalizedContainerPruneDays,
    normalizedMetricHours,
    serializedAutoUpdateDays,
    activityIsDirty,
    automationIsDirty,
    retentionIsDirty,
    setTrackContainerEventsEnabled,
    setCollectContainerMetricsEnabled,
    setHighlightContainerChangesEnabled,
    setDockerDiskUsageNotificationsEnabled,
    setDockerDiskUsageThresholdPercent,
    setDockerDiskUsageUseGlobalDefault,
    setScheduledUpdateCheckEnabled,
    setAutomaticImagePruningEnabled,
    setImagePruneDanglingOnly,
    setTimezone,
    setLogRetentionDays,
    setContainerPruneEnabled,
    setContainerPruneStoppedDays,
    setAutoUpdateEnabled,
    setAutoUpdateWindowStart,
    setAutoUpdateWindowEnd,
    setMetricRetentionHours,
  };
}
