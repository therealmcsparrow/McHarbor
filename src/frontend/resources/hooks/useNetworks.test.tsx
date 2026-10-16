// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import type { PropsWithChildren } from 'react';
import { describe, expect, it } from 'vitest';
import { createTestQueryClient } from '../../test/render';
import { server } from '../../test/server';
import { useCreateNetwork } from './useNetworks';

function wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

const macvlan = {
  name: 'lan',
  driver: 'macvlan',
  options: { parent: 'eth0' },
  ipam: { Config: [{ Subnet: '192.168.1.0/24', IPRange: '192.168.1.200-192.168.1.250' }] },
};

describe('useCreateNetwork', () => {
  it('names the bad IPAM field instead of "invalid request body"', async () => {
    server.use(
      http.post('*/api/networks', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'err.invalid_body',
            error: 'Invalid request body',
            data: { reason: 'invalid_ip_range' },
          },
          { status: 400 },
        ),
      ),
    );
    const { result } = renderHook(() => useCreateNetwork(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(macvlan)).rejects.toThrow(
        'IP range must be a CIDR block inside the subnet, for example 192.168.1.192/27, not a start-end range.',
      );
    });
  });

  it('keeps the server message for a refusal it has no wording for', async () => {
    server.use(
      http.post('*/api/networks', () =>
        HttpResponse.json(
          { success: false, code: 'err.network.create_failed', error: 'Failed to create network' },
          { status: 500 },
        ),
      ),
    );
    const { result } = renderHook(() => useCreateNetwork(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(macvlan)).rejects.toThrow('Failed to create network');
    });
  });

  it("keeps the agent's own words after the host link wording", async () => {
    server.use(
      http.post('*/api/networks', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'err.network.create_failed',
            error: 'Failed to create network',
            detail: 'ip link add: RTNETLINK answers: File exists',
            data: { reason: 'host_link_failed' },
          },
          { status: 500 },
        ),
      ),
    );
    const { result } = renderHook(() => useCreateNetwork(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(macvlan)).rejects.toThrow(
        'The host interface could not be applied. ip link add: RTNETLINK answers: File exists',
      );
    });
  });

  it('says why host interfaces are unavailable, not just that they are', async () => {
    server.use(
      http.post('*/api/networks', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'err.invalid_body',
            error: 'Invalid request body',
            data: { reason: 'host_link_unavailable', unavailable: 'agent_outdated' },
          },
          { status: 400 },
        ),
      ),
    );
    const { result } = renderHook(() => useCreateNetwork(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(macvlan)).rejects.toThrow(
        /Host interfaces cannot be managed here. The agent on this host is too old/,
      );
    });
  });
});
