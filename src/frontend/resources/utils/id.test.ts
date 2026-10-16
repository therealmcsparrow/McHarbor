// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { createClientId } from './id';

describe('createClientId', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('uses randomUUID when available', () => {
    vi.stubGlobal('crypto', { randomUUID: () => 'client-id' });

    expect(createClientId()).toBe('client-id');
  });

  it('builds an RFC 4122 v4 UUID from getRandomValues', () => {
    vi.stubGlobal('crypto', {
      getRandomValues: (bytes: Uint8Array) => {
        bytes.set([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
        return bytes;
      },
    });

    expect(createClientId()).toBe('00010203-0405-4607-8809-0a0b0c0d0e0f');
  });

  it('falls back to timestamp and random data', () => {
    vi.stubGlobal('crypto', undefined);
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-28T12:00:00Z'));
    vi.spyOn(Math, 'random').mockReturnValue(0.5);

    expect(createClientId()).toBe('id-19e6e74aa00-8');
  });
});
