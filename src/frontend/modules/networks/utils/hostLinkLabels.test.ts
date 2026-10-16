// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { isHostLinkLabel, parseHostLinkLabels, withoutHostLinkLabels } from './hostLinkLabels';

describe('parseHostLinkLabels', () => {
  it('reads a static dual-stack link', () => {
    expect(
      parseHostLinkLabels({
        'com.mcharbor.host-link.name': 'mcvlan0',
        'com.mcharbor.host-link.ipv4': '192.168.1.10/24',
        'com.mcharbor.host-link.ipv6': '2001:db8:1::10/64',
        owner: 'me',
      }),
    ).toEqual({ name: 'mcvlan0', ipv4: '192.168.1.10/24', ipv6: '2001:db8:1::10/64' });
  });

  it('reads "auto" as SLAAC, never as an address', () => {
    expect(
      parseHostLinkLabels({ 'com.mcharbor.host-link.name': 'mcvlan0', 'com.mcharbor.host-link.ipv6': 'auto' }),
    ).toEqual({
      name: 'mcvlan0',
      ipv6Auto: true,
    });
  });

  it('is null without a name, whatever else is present', () => {
    expect(parseHostLinkLabels(null)).toBeNull();
    expect(parseHostLinkLabels({ 'com.mcharbor.host-link.ipv4': '192.168.1.10/24' })).toBeNull();
    expect(parseHostLinkLabels({ 'com.mcharbor.host-link.name': '  ' })).toBeNull();
  });
});

describe('withoutHostLinkLabels', () => {
  it('removes only the server-owned labels', () => {
    const rows = [
      { key: 'keep', value: '1' },
      { key: 'com.mcharbor.host-link.name', value: 'mcvlan0' },
      { key: 'com.mcharbor.host-link.ipv6', value: 'auto' },
      { key: 'com.mcharbor.other', value: '2' },
    ];
    expect(withoutHostLinkLabels(rows).map((r) => r.key)).toEqual(['keep', 'com.mcharbor.other']);
    expect(isHostLinkLabel('com.mcharbor.host-link.ipv4')).toBe(true);
    expect(isHostLinkLabel('com.mcharbor.host')).toBe(false);
  });
});
