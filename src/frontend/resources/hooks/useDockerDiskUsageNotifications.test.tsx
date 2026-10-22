// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DockerSystemInfo } from '@core/types/docker';

const GIB = 1024 ** 3;

const mocks = vi.hoisted(() => ({
  warning: vi.fn(),
  info: { driverStatus: [] as string[][] },
  metrics: { disk: { total: 0 }, hostFs: { total: 0 } },
}));

vi.mock('sonner', () => ({ toast: { warning: mocks.warning } }));
vi.mock('./useCurrentEnvironmentActivitySettings', () => ({
  useCurrentEnvironmentActivitySettings: () => ({
    currentId: 'e1',
    currentEnvironment: { name: 'Lab' },
    dockerDiskUsageNotificationsEnabled: true,
    dockerDiskUsageThresholdPercent: 80,
  }),
}));
vi.mock('@resources/hooks/useHostMetrics', () => ({ useHostMetrics: () => ({ data: mocks.metrics }) }));
vi.mock('@resources/hooks/useDockerInfo', () => ({ useDockerInfo: () => ({ data: mocks.info }) }));

import { resolveDiskCapacity, useDockerDiskUsageNotifications } from './useDockerDiskUsageNotifications';

describe('docker disk usage capacity', () => {
  beforeEach(() => {
    mocks.warning.mockClear();
    mocks.info = { driverStatus: [] };
  });

  it('prefers the driver status over the host filesystem', () => {
    const info = { driverStatus: [['Data Space Total', '10 GiB']] } as DockerSystemInfo;
    expect(resolveDiskCapacity(info, 1 * GIB, 100 * GIB)).toBe(10 * GIB);
  });

  it('falls back to the host filesystem on overlay2, which reports no space', () => {
    const info = { driverStatus: [['Backing Filesystem', 'extfs']] } as DockerSystemInfo;
    expect(resolveDiskCapacity(info, 1 * GIB, 100 * GIB)).toBe(100 * GIB);
    expect(resolveDiskCapacity(info, 1 * GIB, 0)).toBeNull();
  });

  it('warns on overlay2 once Docker takes more than the threshold of the host filesystem', () => {
    mocks.metrics = { disk: { total: 90 * GIB }, hostFs: { total: 100 * GIB } };
    renderHook(() => useDockerDiskUsageNotifications());
    expect(mocks.warning).toHaveBeenCalledTimes(1);
  });

  it('stays quiet below the threshold', () => {
    mocks.metrics = { disk: { total: 10 * GIB }, hostFs: { total: 100 * GIB } };
    renderHook(() => useDockerDiskUsageNotifications());
    expect(mocks.warning).not.toHaveBeenCalled();
  });
});
