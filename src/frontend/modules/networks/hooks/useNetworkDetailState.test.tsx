// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useLocation } from 'react-router';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/server';
import { createTestQueryClient } from '../../../test/render';
import { useNetworkDetailState } from './useNetworkDetailState';

function network(name: string) {
  return {
    Id: 'net1',
    Name: name,
    Driver: 'bridge',
    Scope: 'local',
    Internal: false,
    Attachable: false,
    EnableIPv6: false,
    IPAM: { Driver: 'default', Config: [{ Subnet: '172.30.0.0/16', Gateway: '172.30.0.1' }] },
    Labels: {},
    Options: {},
    Containers: {},
  };
}

function renderAt(url: string, name = 'appnet') {
  server.use(
    http.get('*/api/networks/net1', () => HttpResponse.json({ success: true, data: network(name) })),
    http.get('*/api/containers', () => HttpResponse.json({ success: true, data: [] })),
  );
  const client = createTestQueryClient();
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[url]}>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  }
  return renderHook(() => ({ state: useNetworkDetailState('net1', (key) => key), location: useLocation() }), {
    wrapper: Wrapper,
  });
}

describe('useNetworkDetailState ?edit=1', () => {
  it('opens the editor with the network loaded and drops the flag from the URL', async () => {
    const { result } = renderAt('/networks/net1?edit=1');
    await waitFor(() => expect(result.current.state.editing).toBe(true));
    expect(result.current.state.editData?.name).toBe('appnet');
    expect(result.current.location.search).toBe('');
  });

  it('stays on the read-only view without the flag', async () => {
    const { result } = renderAt('/networks/net1');
    await waitFor(() => expect(result.current.state.network?.Name).toBe('appnet'));
    expect(result.current.state.editing).toBe(false);
  });

  it('never opens the editor for a built-in network', async () => {
    const { result } = renderAt('/networks/net1?edit=1', 'bridge');
    await waitFor(() => expect(result.current.location.search).toBe(''));
    expect(result.current.state.editing).toBe(false);
  });
});
