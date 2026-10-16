// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useBatchProgressOperation } from './useBatchProgressOperation';

async function failWith(rejection: unknown) {
  const { result } = renderHook(() => useBatchProgressOperation());
  let outcome: Awaited<ReturnType<typeof result.current.runBatchOperation<string>>> | undefined;
  await act(async () => {
    outcome = await result.current.runBatchOperation<string>({
      title: 't',
      description: 'd',
      actionLabel: 'a',
      items: ['one'],
      getKey: (item) => item,
      getLabel: (item) => item,
      execute: () => Promise.reject(rejection),
    });
  });
  return outcome?.failedItems[0]?.error.message;
}

describe('useBatchProgressOperation failure messages', () => {
  it('keeps the text of a rejection that is a plain string', async () => {
    expect(await failWith('the host refused the stop')).toBe('the host refused the stop');
  });

  it('falls back to the translated operation-failed message, never hardcoded English', async () => {
    expect(await failWith({ unexpected: true })).toBe('Operation failed');
  });
});
