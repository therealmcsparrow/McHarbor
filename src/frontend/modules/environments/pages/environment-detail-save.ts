// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { normalizeTimezone } from '@resources/utils/timezones';
import type { EnvironmentDetailTab, useEnvironmentDetailState } from '../hooks/useEnvironmentDetailState';

type DetailState = ReturnType<typeof useEnvironmentDetailState>;

export function isSaveTab(tab: EnvironmentDetailTab): boolean {
  return tab === 'activity' || tab === 'automation' || tab === 'retention';
}

export function isTabDirty(tab: EnvironmentDetailTab, state: DetailState): boolean {
  if (tab === 'activity') return state.activityIsDirty;
  if (tab === 'automation') return state.automationIsDirty;
  if (tab === 'retention') return state.retentionIsDirty;
  return false;
}

export function buildSavePayload(tab: EnvironmentDetailTab, state: DetailState) {
  if (tab === 'activity') {
    return {
      trackContainerEventsEnabled: state.trackContainerEventsEnabled,
      collectContainerMetricsEnabled: state.collectContainerMetricsEnabled,
      highlightContainerChangesEnabled: state.highlightContainerChangesEnabled,
      dockerDiskUsageNotificationsEnabled: state.dockerDiskUsageNotificationsEnabled,
      dockerDiskUsageThresholdPercent: state.normalizedThreshold,
      dockerDiskUsageUseGlobalDefault: state.dockerDiskUsageUseGlobalDefault,
    };
  }
  if (tab === 'automation') {
    return {
      scheduledUpdateCheckEnabled: state.scheduledUpdateCheckEnabled,
      automaticImagePruningEnabled: state.automaticImagePruningEnabled,
      imagePruneDanglingOnly: state.imagePruneDanglingOnly,
      timezone: normalizeTimezone(state.timezone),
    };
  }
  return {
    logRetentionDays: state.normalizedLogRetention,
    containerPruneEnabled: state.containerPruneEnabled,
    containerPruneStoppedDays: state.normalizedContainerPruneDays,
    metricRetentionHours: state.normalizedMetricHours,
  };
}
