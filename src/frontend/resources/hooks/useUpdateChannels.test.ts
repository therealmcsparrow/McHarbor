// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { createElement, type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../test/permissions';
import { server } from '../../test/server';
import { UPDATE_CHANNELS_KEY, useSetUpdateChannel, useUpdateChannels } from './useUpdateChannels';

const initial = {
  mcharborPrereleases: true,
  agentPrereleases: true,
  pluginPrereleases: false,
  mcharborAutoPrereleases: false,
  agentAutoPrereleases: false,
};

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
}

describe('useUpdateChannels', () => {
  const reads = vi.fn();
  const puts = vi.fn();
  let stored = { ...initial };

  beforeEach(() => {
    reads.mockReset();
    puts.mockReset();
    stored = { ...initial };
    server.use(
      http.get('*/api/updates/channels', () => {
        reads();
        return HttpResponse.json({ success: true, data: stored });
      }),
      http.put('*/api/updates/channels', async ({ request }) => {
        const body = (await request.json()) as Record<string, boolean>;
        puts(body);
        stored = { ...stored, ...body };
        return HttpResponse.json({ success: true, data: stored });
      }),
    );
  });

  it('does not request the settings without updates.view, and is not an error', async () => {
    grantPermissions([]);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrap = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
    const { result } = renderHook(() => useUpdateChannels(), { wrapper: wrap });
    expect(result.current.canView).toBe(false);
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(client.getQueryState(UPDATE_CHANNELS_KEY)?.status).toBe('pending');
    expect(result.current.isError).toBe(false);
    expect(result.current.settings).toBeUndefined();
    expect(reads).not.toHaveBeenCalled();
  });

  it('reads the five settings with updates.view', async () => {
    grantPermissions(['updates.view']);
    const { result } = renderHook(() => useUpdateChannels(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.settings).toEqual(initial));
  });

  it('a refused read is reported as isError, never thrown', async () => {
    grantPermissions(['updates.view']);
    server.use(
      http.get('*/api/updates/channels', () => HttpResponse.json({ success: false, error: 'boom' }, { status: 500 })),
    );
    const { result } = renderHook(() => useUpdateChannels(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('the mutation sends only the changed field', async () => {
    grantPermissions(['updates.view', 'updates.manage']);
    const { result } = renderHook(() => ({ set: useSetUpdateChannel(), read: useUpdateChannels() }), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.read.settings).toBeDefined());
    await act(async () => {
      await result.current.set.mutateAsync({ pluginPrereleases: true });
    });
    expect(puts).toHaveBeenCalledWith({ pluginPrereleases: true });
    expect(Object.keys(puts.mock.calls[0]![0] as object)).toEqual(['pluginPrereleases']);
    await waitFor(() => expect(result.current.read.settings?.pluginPrereleases).toBe(true));
  });
});
