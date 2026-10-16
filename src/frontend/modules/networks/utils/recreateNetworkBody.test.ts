// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { NetworkEditData } from '../hooks/useNetworkDetailState';
import { EMPTY_HOST_LINK, hostLinkFormOf } from './hostLinkForm';
import { recreateNetworkBody } from './recreateNetworkBody';

const existingLabels = {
  keep: '1',
  'com.mcharbor.host-link.name': 'mcvlan0',
  'com.mcharbor.host-link.ipv4': '192.168.1.250/24',
  'com.mcharbor.host-link.ipv6': 'auto',
};

const edit: NetworkEditData = {
  name: 'lan',
  driver: 'macvlan',
  internal: false,
  attachable: false,
  enableIPv6: true,
  ipamDriver: 'default',
  ipamConfig: [
    { subnet: '192.168.1.0/24', gateway: '192.168.1.1', ipRange: '', auxAddresses: { printer: '192.168.1.5' } },
    { subnet: '', gateway: '', ipRange: '' },
  ],
  labels: [{ key: 'keep', value: '1' }],
  options: [{ key: 'parent', value: 'eth0' }],
  hostLink: hostLinkFormOf(existingLabels),
};

describe('recreateNetworkBody', () => {
  it('carries IPv6, the auxiliary reservations and an unchanged host link through an edit', () => {
    const body = recreateNetworkBody(edit);
    expect(body.enableIPv6).toBe(true);
    expect(body.hostLink).toEqual({ name: 'mcvlan0', ipv4: '192.168.1.250/24', ipv6Auto: true });
    expect(body.ipam?.Config).toEqual([
      {
        Subnet: '192.168.1.0/24',
        Gateway: '192.168.1.1',
        IPRange: undefined,
        AuxiliaryAddresses: { printer: '192.168.1.5' },
      },
    ]);
    expect(body.labels).toEqual({ keep: '1' });
  });

  it('sends a renamed or newly added host interface from the edit form', () => {
    const renamed = recreateNetworkBody({
      ...edit,
      hostLink: { ...edit.hostLink, name: 'lan-docker', ipv6Auto: false, ipv6: '' },
    });
    expect(renamed.hostLink).toEqual({ name: 'lan-docker', ipv4: '192.168.1.250/24' });
    const added = recreateNetworkBody({
      ...edit,
      hostLink: { enabled: true, name: 'shim0', ipv4: '192.168.1.240/24', ipv6: '', ipv6Auto: false },
    });
    expect(added.hostLink).toEqual({ name: 'shim0', ipv4: '192.168.1.240/24' });
  });

  it('drops the host interface when it is switched off or the network stops being macvlan', () => {
    expect(recreateNetworkBody({ ...edit, hostLink: EMPTY_HOST_LINK }).hostLink).toBeUndefined();
    expect(recreateNetworkBody({ ...edit, driver: 'bridge' }).hostLink).toBeUndefined();
  });

  it('reads no host interface for a network that never had one', () => {
    expect(hostLinkFormOf({ keep: '1' })).toEqual(EMPTY_HOST_LINK);
  });
});
