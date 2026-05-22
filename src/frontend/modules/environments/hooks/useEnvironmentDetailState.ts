// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { normalizeTimezone } from '../timezones';
import type { EnvironmentInfo } from './useEnvironments';

export type EnvironmentDetailTab = 'overview' | 'activity' | 'automation';

function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) {
    return 80;
  }

  return Math.min(100, Math.max(1, Math.round(value)));
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
  const [scheduledUpdateCheckEnabled, setScheduledUpdateCheckEnabled] = useState(false);
  const [automaticImagePruningEnabled, setAutomaticImagePruningEnabled] = useState(false);
  const [timezone, setTimezone] = useState('UTC');

  useEffect(() => {
    if (!env) {
      return;
    }

    setTrackContainerEventsEnabled(env.trackContainerEventsEnabled);
    setCollectContainerMetricsEnabled(env.collectContainerMetricsEnabled);
    setHighlightContainerChangesEnabled(env.highlightContainerChangesEnabled);
    setDockerDiskUsageNotificationsEnabled(env.dockerDiskUsageNotificationsEnabled);
    setDockerDiskUsageThresholdPercent(String(env.dockerDiskUsageThresholdPercent));
    setScheduledUpdateCheckEnabled(env.scheduledUpdateCheckEnabled);
    setAutomaticImagePruningEnabled(env.automaticImagePruningEnabled);
    setTimezone(normalizeTimezone(env.timezone));
  }, [env]);

  const normalizedThreshold = clampThreshold(Number.parseInt(dockerDiskUsageThresholdPercent, 10));
  const activityIsDirty = env
    ? trackContainerEventsEnabled !== env.trackContainerEventsEnabled ||
      collectContainerMetricsEnabled !== env.collectContainerMetricsEnabled ||
      highlightContainerChangesEnabled !== env.highlightContainerChangesEnabled ||
      dockerDiskUsageNotificationsEnabled !== env.dockerDiskUsageNotificationsEnabled ||
      normalizedThreshold !== env.dockerDiskUsageThresholdPercent
    : false;
  const automationIsDirty = env
    ? scheduledUpdateCheckEnabled !== env.scheduledUpdateCheckEnabled ||
      automaticImagePruningEnabled !== env.automaticImagePruningEnabled ||
      timezone !== normalizeTimezone(env.timezone)
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
    scheduledUpdateCheckEnabled,
    automaticImagePruningEnabled,
    timezone,
    normalizedThreshold,
    activityIsDirty,
    automationIsDirty,
    setTrackContainerEventsEnabled,
    setCollectContainerMetricsEnabled,
    setHighlightContainerChangesEnabled,
    setDockerDiskUsageNotificationsEnabled,
    setDockerDiskUsageThresholdPercent,
    setScheduledUpdateCheckEnabled,
    setAutomaticImagePruningEnabled,
    setTimezone,
  };
}
