// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HOST_LOG_POLL_MS, useHostLogs } from './useHostLogs';

const get = vi.hoisted(() => vi.fn());
vi.mock('@core/api/client', () => ({ api: { get }, envelopeStatus: () => undefined }));

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('useHostLogs qemu guests', () => {
  beforeEach(() => {
    get.mockReset();
    get.mockImplementation((_path: string, params: Record<string, string>) =>
      Promise.resolve({
        success: true,
        data: params.target
          ? { source: 'qemu', tail: 200, lines: ['qemu: started'], notices: [], availableFiles: [] }
          : {
              source: 'qemu',
              tail: 1,
              lines: [],
              notices: [],
              availableFiles: ['legacy.log'],
              targets: [{ id: 'vm-cs1abc', label: 'Home Assistant' }],
            },
      }),
    );
  });

  it('lists the host guest units and legacy logs without reading any guest yet', async () => {
    const { result } = renderHook(() => useHostLogs('env1', 'qemu', 200, ''), { wrapper });
    await waitFor(() => expect(result.current.guests).toHaveLength(2));
    expect(result.current.guests).toEqual([
      { id: 'vm-cs1abc', label: 'Home Assistant' },
      { id: 'legacy', label: 'legacy' },
    ]);
    expect(get).toHaveBeenCalledTimes(1);
    expect(get.mock.calls[0]?.[1]).not.toHaveProperty('target');
  });

  it('keeps the guest list once a guest is picked and reads that guest', async () => {
    const { result } = renderHook(() => useHostLogs('env1', 'qemu', 200, 'vm-cs1abc'), { wrapper });
    await waitFor(() => expect(result.current.lines).toEqual(['qemu: started']));
    expect(result.current.guests.map((g) => g.id)).toContain('vm-cs1abc');
    expect(get.mock.calls.some((call) => call[1]?.target === 'vm-cs1abc')).toBe(true);
  });
});

describe('useHostLogs live polling', () => {
  beforeEach(() => {
    get.mockReset();
    get.mockResolvedValue({
      success: true,
      data: { source: 'system', tail: 200, lines: ['a'], notices: [], availableFiles: [] },
    });
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => vi.useRealTimers());

  it('reads once when Live is off, however long it waits', async () => {
    renderHook(() => useHostLogs('env1', 'system', 200, '', false), { wrapper });
    await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(HOST_LOG_POLL_MS * 3);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('re-reads on the poll interval while Live is on', async () => {
    renderHook(() => useHostLogs('env1', 'system', 200, '', true), { wrapper });
    await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(HOST_LOG_POLL_MS + 100);
    await waitFor(() => expect(get.mock.calls.length).toBeGreaterThanOrEqual(2));
  });
});
