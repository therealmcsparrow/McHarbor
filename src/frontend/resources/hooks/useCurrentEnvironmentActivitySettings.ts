// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEnvironmentStore } from '@resources/stores/environment';

const DEFAULT_ACTIVITY_SETTINGS = {
  trackContainerEventsEnabled: true,
  collectContainerMetricsEnabled: true,
  highlightContainerChangesEnabled: true,
  dockerDiskUsageNotificationsEnabled: true,
  dockerDiskUsageThresholdPercent: 80,
};

export function useCurrentEnvironmentActivitySettings() {
  const currentId = useEnvironmentStore((state) => state.currentId);
  const currentEnvironment = useEnvironmentStore((state) =>
    state.environments.find((environment) => environment.id === state.currentId)
  );

  return {
    currentId,
    currentEnvironment,
    trackContainerEventsEnabled:
      currentEnvironment?.trackContainerEventsEnabled ?? DEFAULT_ACTIVITY_SETTINGS.trackContainerEventsEnabled,
    collectContainerMetricsEnabled:
      currentEnvironment?.collectContainerMetricsEnabled ?? DEFAULT_ACTIVITY_SETTINGS.collectContainerMetricsEnabled,
    highlightContainerChangesEnabled:
      currentEnvironment?.highlightContainerChangesEnabled ?? DEFAULT_ACTIVITY_SETTINGS.highlightContainerChangesEnabled,
    dockerDiskUsageNotificationsEnabled:
      currentEnvironment?.dockerDiskUsageNotificationsEnabled ?? DEFAULT_ACTIVITY_SETTINGS.dockerDiskUsageNotificationsEnabled,
    dockerDiskUsageThresholdPercent:
      currentEnvironment?.dockerDiskUsageThresholdPercent ?? DEFAULT_ACTIVITY_SETTINGS.dockerDiskUsageThresholdPercent,
  };
}
