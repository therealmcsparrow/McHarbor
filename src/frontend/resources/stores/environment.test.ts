// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { useEnvironmentStore } from './environment';

const baseEnvironment = {
  id: 'env-1',
  name: 'Local',
  orchestratorType: 'docker' as const,
  connectionType: 'socket',
  isDefault: true,
  dockerVersion: '26.0.0',
  k8sVersion: null,
  scheduledUpdateCheckEnabled: true,
  automaticImagePruningEnabled: false,
  trackContainerEventsEnabled: true,
  collectContainerMetricsEnabled: true,
  highlightContainerChangesEnabled: true,
  dockerDiskUsageNotificationsEnabled: false,
  dockerDiskUsageThresholdPercent: 80,
};

describe('useEnvironmentStore', () => {
  beforeEach(() => {
    useEnvironmentStore.setState({ currentId: '', environments: [] });
  });

  it('sets the current environment and environment list', () => {
    useEnvironmentStore.getState().setCurrentId('env-1');
    useEnvironmentStore.getState().setEnvironments([baseEnvironment]);

    expect(useEnvironmentStore.getState().currentId).toBe('env-1');
    expect(useEnvironmentStore.getState().environments).toEqual([baseEnvironment]);
  });

  it('adds and updates environments by ID', () => {
    useEnvironmentStore.getState().upsertEnvironment(baseEnvironment);
    useEnvironmentStore.getState().upsertEnvironment({ ...baseEnvironment, name: 'Renamed', isDefault: false });

    expect(useEnvironmentStore.getState().environments).toEqual([
      { ...baseEnvironment, name: 'Renamed', isDefault: false },
    ]);
  });
});
