// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { normalizeTimezone } from '@resources/utils/timezones';
import type { EnvironmentInfo } from './useEnvironments';
import {
  ALL_DAYS,
  clampDayCount,
  clampHours,
  clampThreshold,
  parseSelectedDays,
  serializeSelectedDays,
  type AutoUpdateDay,
} from './environmentDetailHelpers';

export type { AutoUpdateDay };

export const environmentDetailTabs = [
  'overview',
  'activity',
  'automation',
  'retention',
  'access',
  'host',
  'terminal',
  'logs',
  'docker',
  'processes',
  'services',
  'updates',
] as const;

export type EnvironmentDetailTab = (typeof environmentDetailTabs)[number];

export function useEnvironmentDetailState(env?: EnvironmentInfo) {
  const [tokenDialogOpen, setTokenDialogOpen] = useState(false);
  const [regeneratedToken, setRegeneratedToken] = useState('');
  const [regeneratedPin, setRegeneratedPin] = useState('');
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

  const [syncedEnv, setSyncedEnv] = useState<typeof env>(undefined);
  if (env !== syncedEnv) {
    setSyncedEnv(env);
    if (env) {
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
    }
  }

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
    regeneratedPin,
    setRegeneratedPin,
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
