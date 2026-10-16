// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { ContainerInfo } from '@core/types/docker';
import { firstPublicPort, ipSortKey, updateSortValue, uptimeSeconds } from './container-sort-values';

function withIp(ip: string): ContainerInfo {
  return { NetworkSettings: { Networks: { bridge: { IPAddress: ip } } } } as unknown as ContainerInfo;
}

function order<T>(items: T[], key: (item: T) => string | number | null): T[] {
  return [...items].sort((a, b) => {
    const ka = key(a);
    const kb = key(b);
    if (ka === kb) return 0;
    if (ka === null) return 1;
    if (kb === null) return -1;
    return ka < kb ? -1 : 1;
  });
}

describe('container sort values', () => {
  it('orders uptime by elapsed time rather than by the text', () => {
    const statuses = ['Up 2 minutes', 'Up 3 days (healthy)', 'Up About an hour', 'Exited (0) 1 hour ago'];
    expect(order(statuses, uptimeSeconds)).toEqual([
      'Up 2 minutes',
      'Up About an hour',
      'Up 3 days (healthy)',
      'Exited (0) 1 hour ago',
    ]);
    expect(uptimeSeconds('Up Less than a second')).toBe(0);
  });

  it('orders IPv4 addresses numerically and leaves address-less containers last', () => {
    const rows = [withIp('172.18.0.10'), withIp(''), withIp('172.18.0.9'), withIp('10.0.0.200')];
    const sorted = order(rows, ipSortKey).map((c) => c.NetworkSettings.Networks.bridge?.IPAddress);
    expect(sorted).toEqual(['10.0.0.200', '172.18.0.9', '172.18.0.10', '']);
  });

  it('orders ports by the lowest published port', () => {
    const rows = [
      [{ PrivatePort: 80, PublicPort: 8080, Type: 'tcp' }],
      [
        { PrivatePort: 443, PublicPort: 443, Type: 'tcp' },
        { PrivatePort: 80, PublicPort: 9000, Type: 'tcp' },
      ],
      [{ PrivatePort: 5432, Type: 'tcp' }],
    ];
    expect(order(rows, firstPublicPort).map((p) => firstPublicPort(p))).toEqual([443, 8080, null]);
  });

  it('ranks update results error, current, available', () => {
    expect(updateSortValue({ containerId: 'a', updateAvailable: false, error: 'x' })).toBe(0);
    expect(updateSortValue({ containerId: 'a', updateAvailable: false })).toBe(1);
    expect(updateSortValue({ containerId: 'a', updateAvailable: true })).toBe(2);
    expect(updateSortValue(undefined)).toBeNull();
  });
});
