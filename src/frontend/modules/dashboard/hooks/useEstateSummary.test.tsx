// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { createTestQueryClient } from '../../../test/render';
import { server } from '../../../test/server';
import { useEstateSummary } from './useEstateSummary';

const vmSource = {
  plugin: 'vm-plugin',
  kind: 'estate-source',
  metric: 'vms',
  path: '/api/guests',
  environmentField: 'hostEnv',
};

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

describe('useEstateSummary', () => {
  const listed = vi.fn();

  beforeEach(() => {
    listed.mockReset();
    server.use(
      http.get('*/api/dashboard/summary', () => HttpResponse.json({ success: true, data: { environments: 3 } })),
      http.get('*/api/updates/agents', () => HttpResponse.json({ success: true, data: { agents: [] } })),
      http.get('*/api/scans/summary', () => HttpResponse.json({ success: true, data: { needsReview: 0 } })),
      http.get('*/api/guests', () => {
        listed();
        return HttpResponse.json({
          success: true,
          data: [{ hostEnv: 'e1' }, { hostEnv: 'e1' }, { hostEnv: 'e2' }],
        });
      }),
    );
  });
  afterEach(() => clearPluginCapabilities());

  it('counts guests from the route and field the estate source declares', async () => {
    givePluginCapabilities([vmSource]);
    const { result } = renderHook(() => useEstateSummary(), { wrapper });
    await waitFor(() => expect(result.current.data?.vms).toBe(3));
    expect(result.current.data?.vmEnvironments).toBe(2);
    expect(listed).toHaveBeenCalled();
  });

  it('leaves the guest segment absent, and asks nobody, when no plugin declares it', async () => {
    givePluginCapabilities([vmSource], ['vm-plugin']);
    const { result } = renderHook(() => useEstateSummary(), { wrapper });
    await waitFor(() => expect(result.current.data?.environments).toBe(3));
    expect(result.current.data?.vms).toBeUndefined();
    expect(listed).not.toHaveBeenCalled();
  });

  it('counts agents behind from the shared agent inventory route', async () => {
    server.use(
      http.get('*/api/updates/agents', () =>
        HttpResponse.json({
          success: true,
          data: { targetVersion: '2026.09.01', agents: [{ outdated: true }, { outdated: true }, { outdated: false }] },
        }),
      ),
    );
    const { result } = renderHook(() => useEstateSummary(), { wrapper });
    await waitFor(() => expect(result.current.data?.agentsBehind).toBe(2));
  });
});
