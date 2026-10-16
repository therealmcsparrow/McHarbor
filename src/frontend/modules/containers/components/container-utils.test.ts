// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import type { ContainerInfo } from '@core/types/docker';
import {
  HEALTH_COLORS,
  STATE_VARIANTS,
  formatPorts,
  getAutoUpdate,
  getContainerIP,
  getContainerWebUrl,
  getPublicPorts,
  getStackName,
  parseHealth,
  parseUptime,
} from './container-utils';

function container(overrides: Partial<ContainerInfo> = {}): ContainerInfo {
  return {
    Id: 'id',
    Names: ['/web'],
    Image: 'nginx',
    ImageID: 'image-id',
    Command: '',
    Created: 0,
    Ports: [],
    Labels: {},
    State: 'running',
    Status: 'Up 3 hours (healthy)',
    NetworkSettings: { Networks: {} },
    Mounts: [],
    ...overrides,
  };
}

describe('container-utils', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { hostname: 'mcharbor.local' },
    });
  });

  it('exports state and health maps', () => {
    expect(STATE_VARIANTS.running).toBe('success');
    expect(STATE_VARIANTS.dead).toBe('destructive');
    expect(HEALTH_COLORS.healthy).toBe('text-success');
  });

  it('parses health and uptime from Docker status text', () => {
    expect(parseHealth('Up 2 minutes (healthy)')).toBe('healthy');
    expect(parseHealth('Up 2 minutes')).toBeNull();
    expect(parseUptime('Up About an hour (healthy)')).toBe('1 hour');
    expect(parseUptime('Up Less than a second')).toBe('<1 second');
    expect(parseUptime('Exited 1 minute ago')).toBe('-');
  });

  it('formats public ports and removes duplicates', () => {
    const ports = [
      { PrivatePort: 80, PublicPort: 8080, Type: 'tcp' },
      { PrivatePort: 80, PublicPort: 8080, Type: 'tcp' },
      { PrivatePort: 53, Type: 'udp' },
    ];

    expect(getPublicPorts(ports)).toEqual(['8080:80/tcp']);
    expect(formatPorts(ports)).toBe('8080:80/tcp');
    expect(formatPorts([])).toBe('-');
  });

  it('reads container IP and stack metadata', () => {
    expect(getContainerIP(container())).toBe('-');
    expect(
      getContainerIP(
        container({
          NetworkSettings: {
            Networks: {
              bridge: {
                IPAddress: '172.17.0.2',
                IPAMConfig: null,
                Links: null,
                Aliases: null,
                NetworkID: '',
                EndpointID: '',
                Gateway: '',
                IPPrefixLen: 16,
                IPv6Gateway: '',
                GlobalIPv6Address: '',
                MacAddress: '',
              },
            },
          },
        }),
      ),
    ).toBe('172.17.0.2');
    expect(getStackName(container({ StackName: 'direct' }))).toBe('direct');
    expect(getStackName(container({ Labels: { 'com.docker.compose.project': 'compose' } }))).toBe('compose');
    expect(getStackName(container())).toBeNull();
  });

  it('detects auto-update labels and builds likely web URLs', () => {
    expect(getAutoUpdate(container({ Labels: { 'com.centurylinklabs.watchtower.enable': 'true' } }))).toBe(true);
    expect(getAutoUpdate(container({ Labels: { 'mcharbor.auto-update': 'enabled' } }))).toBe(true);
    expect(getAutoUpdate(container())).toBe(false);

    expect(getContainerWebUrl([])).toBeNull();
    expect(getContainerWebUrl([{ PrivatePort: 443, PublicPort: 9443, Type: 'tcp' }])).toBe(
      'https://mcharbor.local:9443',
    );
    expect(getContainerWebUrl([{ PrivatePort: 1234, PublicPort: 1234, Type: 'tcp' }])).toBe(
      'http://mcharbor.local:1234',
    );
    expect(getContainerWebUrl([{ PrivatePort: 53, PublicPort: 53, Type: 'udp' }])).toBeNull();
  });
});
