// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { ipv4SortKey } from './ip-sort';

describe('ipv4SortKey', () => {
  it('orders addresses numerically under a string compare', () => {
    const sorted = ['172.18.0.10', '10.0.0.200', '172.18.0.9'].sort((a, b) =>
      (ipv4SortKey(a) ?? '').localeCompare(ipv4SortKey(b) ?? ''),
    );
    expect(sorted).toEqual(['10.0.0.200', '172.18.0.9', '172.18.0.10']);
  });

  it('returns null for a missing address and leaves non-IPv4 text alone', () => {
    expect(ipv4SortKey(undefined)).toBeNull();
    expect(ipv4SortKey('-')).toBeNull();
    expect(ipv4SortKey('fe80::1')).toBe('fe80::1');
  });
});
