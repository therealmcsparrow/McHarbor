// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { AgentRouting, ClusterReadiness } from '../hooks/useCluster';
import { HAReadinessChecklist } from './HAReadinessChecklist';

const activeActive: ClusterReadiness = {
  target: 'active-active',
  ready: false,
  enabled: false,
  gates: [
    { id: 'externalDatabase', status: 'pass', detail: '', values: ['postgres'] },
    { id: 'agentRouting', status: 'fail', detail: '' },
    { id: 'perNodeState', status: 'warn', detail: '' },
  ],
};

const unproven: AgentRouting = {
  nodeId: 'node-b',
  canForward: true,
  owners: [{ envId: 'env-1', nodeId: 'node-a', advertiseUrl: 'http://node-a:8705', expiresAt: '' }],
  proof: { status: 'unproven' },
};

const proven: AgentRouting = {
  ...unproven,
  proof: { status: 'proven', nodeId: 'node-a', envId: 'env-1', at: '2026-09-27T01:00:00Z' },
};

const activePassive: ClusterReadiness = {
  target: 'active-passive',
  ready: true,
  enabled: true,
  gates: [{ id: 'agentRouting', status: 'pass', detail: '' }],
};

describe('HA readiness checklist', () => {
  const targets = vi.fn();
  const checks = vi.fn();
  let routing: AgentRouting;
  beforeEach(() => {
    targets.mockReset();
    checks.mockReset();
    routing = unproven;
    grantPermissions(['cluster.view']);
    server.use(
      http.get('*/api/cluster/readiness', ({ request }) => {
        const target = new URL(request.url).searchParams.get('target');
        targets(target);
        return HttpResponse.json({ success: true, data: target === 'active-passive' ? activePassive : activeActive });
      }),
      http.get('*/api/cluster/agent-routing', () => HttpResponse.json({ success: true, data: routing })),
      http.post('*/api/cluster/agent-routing-check', () => {
        checks();
        routing = proven;
        return HttpResponse.json({ success: true, data: proven });
      }),
    );
  });

  it('shows active/active as blocked by agent routing and disabled', async () => {
    renderWithProviders(<HAReadinessChecklist />);
    expect(await screen.findByText('At least one gate fails for this target.')).toBeInTheDocument();
    expect(screen.getByText(/Active\/active stays disabled/)).toBeInTheDocument();
    expect(screen.getByText('Remote agent routing')).toBeInTheDocument();
    expect(screen.getByText(/the node-to-node hop is unconfigured/)).toBeInTheDocument();
    expect(screen.getByText('Fail')).toBeInTheDocument();
    expect(screen.getByText('Warning')).toBeInTheDocument();
    expect(targets).toHaveBeenCalledWith('active-active');
  });

  it('asks for the active/passive target when it is chosen', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HAReadinessChecklist />);
    await screen.findByText('At least one gate fails for this target.');
    await user.click(screen.getByRole('button', { name: 'Active/passive' }));
    expect(await screen.findByText('Every gate passes for this target.')).toBeInTheDocument();
    expect(targets).toHaveBeenCalledWith('active-passive');
    expect(screen.queryByText(/Active\/active stays disabled/)).toBeNull();
  });

  it('says nothing has been forwarded yet, and proves it on request', async () => {
    grantPermissions(['cluster.view', 'cluster.manage']);
    const user = userEvent.setup();
    renderWithProviders(<HAReadinessChecklist />);
    expect(await screen.findByText(/Nothing has been forwarded yet/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Prove cross-node routing' }));
    expect(checks).toHaveBeenCalledTimes(1);
    expect(
      await screen.findByText(
        'A call to environment env-1 was executed by node node-a, so forwarding works from here.',
      ),
    ).toBeInTheDocument();
  });

  it('disables the routing check without cluster.manage rather than hiding it', async () => {
    renderWithProviders(<HAReadinessChecklist />);
    const button = await screen.findByRole('button', { name: 'Prove cross-node routing' });
    expect(button).toBeDisabled();
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)));
    expect(checks).not.toHaveBeenCalled();
  });

  it('offers no routing proof on the active/passive target', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HAReadinessChecklist />);
    await screen.findByText('At least one gate fails for this target.');
    await user.click(screen.getByRole('button', { name: 'Active/passive' }));
    await screen.findByText('Every gate passes for this target.');
    expect(screen.queryByRole('button', { name: 'Prove cross-node routing' })).toBeNull();
  });

  it('names cluster.view and asks nothing without it', async () => {
    grantPermissions([]);
    renderWithProviders(<HAReadinessChecklist />);
    expect(screen.getByText('Requires the cluster.view permission')).toBeInTheDocument();
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)));
    expect(targets).not.toHaveBeenCalled();
    expect(screen.queryByText('Remote agent routing')).toBeNull();
  });
});
