// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useStableListKeys } from './useStableListKeys';

let counter = 0;

vi.mock('../utils/id', () => ({
  createClientId: () => `id-${++counter}`,
}));

describe('useStableListKeys', () => {
  beforeEach(() => {
    counter = 0;
  });

  it('keeps keys stable by fingerprint across reorders', () => {
    const { result, rerender } = renderHook(({ items }) => useStableListKeys(items, (item) => item.id), {
      initialProps: { items: [{ id: 'a' }, { id: 'b' }] },
    });

    const first = result.current;
    rerender({ items: [{ id: 'b' }, { id: 'a' }] });

    expect(result.current).toEqual([first[1], first[0]]);
  });

  it('falls back to index matching and creates keys for new rows', () => {
    const { result, rerender } = renderHook(({ items }) => useStableListKeys(items), {
      initialProps: { items: ['first'] },
    });

    const first = result.current[0];
    rerender({ items: ['changed', 'second'] });

    expect(result.current[0]).toBe(first);
    expect(result.current[1]).toBe('row-id-2');
  });
});
