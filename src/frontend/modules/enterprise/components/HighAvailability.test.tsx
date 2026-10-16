// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { ClusterInstance, ClusterStatus } from '../hooks/useCluster';
import { ClusterInstancesTable } from './ClusterInstancesTable';
import { StandbyBanner } from './StandbyBanner';

const node = (
  id: string,
  haRole: ClusterInstance['haRole'],
  extra: Partial<ClusterInstance> = {},
): ClusterInstance => ({
  id,
  hostname: `${id}.lan`,
  version: '2026.09.01',
  build: '202609262200',
  haRole,
  state: 'running',
  advertiseUrl: '',
  agents: 0,
  startedAt: '2026-09-26T20:00:00Z',
  heartbeatAt: new Date().toISOString(),
  alive: true,
  self: false,
  ...extra,
});

const status = (instances: ClusterInstance[], activeConflict?: string[]): ClusterStatus => ({
  nodeId: 'node-a',
  databaseDriver: 'postgres',
  roles: {},
  database: { reachable: true },
  serverTime: '2026-09-26T20:00:00Z',
  instances,
  activeConflict,
  role: 'active',
  state: 'running',
  mode: 'active-passive',
  ready: true,
  checks: [],
  pluginStore: 'database',
});

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

function row(id: string) {
  return screen.getByText(id).closest('tr') as HTMLElement;
}

describe('High availability controls', () => {
  const failovers = vi.fn();
  beforeEach(() => {
    failovers.mockReset();
    grantPermissions(['cluster.view', 'cluster.manage']);
    server.use(
      http.get('*/api/table-views/enterprise-ha-nodes', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
    );
  });

  it('hands over only after confirmation, and sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/cluster/failover', async ({ request }) => {
        failovers(await request.json());
        await gate;
        return HttpResponse.json({ success: true, data: { requested: true, nodeId: 'node-b' } });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable status={status([node('node-a', 'active', { self: true }), node('node-b', 'standby')])} />,
    );
    await user.click(within(row('node-b')).getByRole('button', { name: /Make active/ }));
    expect(failovers).not.toHaveBeenCalled();
    const confirm = within(screen.getByRole('dialog')).getByRole('button', { name: 'Make active' });
    await user.dblClick(confirm);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(failovers).toHaveBeenCalledTimes(1);
    expect(failovers).toHaveBeenCalledWith({ targetNodeId: 'node-b' });
  });

  it('keeps the dialog and the server message when the handover is refused', async () => {
    server.use(
      http.post('*/api/cluster/failover', () =>
        HttpResponse.json({ success: false, error: 'node-b stopped answering heartbeats' }, { status: 404 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable status={status([node('node-a', 'active'), node('node-b', 'standby')])} />,
    );
    await user.click(within(row('node-b')).getByRole('button', { name: /Make active/ }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Make active' }));
    expect(await screen.findByText('node-b stopped answering heartbeats')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('is disabled, naming cluster.manage, without it', async () => {
    grantPermissions(['cluster.view']);
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable status={status([node('node-a', 'active'), node('node-b', 'standby')])} />,
    );
    const drain = within(row('node-a')).getByRole('button', { name: /Drain/ });
    expect(drain).toBeDisabled();
    expect(within(row('node-b')).getByRole('button', { name: /Make active/ })).toBeDisabled();
    await user.hover(drain.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the cluster.manage permission')).not.toHaveLength(0);
  });

  it('refuses to act through a split brain', () => {
    renderWithProviders(
      <ClusterInstancesTable
        status={status(
          [node('node-a', 'active'), node('node-b', 'active'), node('node-c', 'standby')],
          ['node-a', 'node-b'],
        )}
      />,
    );
    expect(within(row('node-c')).getByRole('button', { name: /Make active/ })).toBeDisabled();
    expect(within(row('node-a')).getByRole('button', { name: /Drain/ })).toBeDisabled();
  });

  it('renders through the shared table layout and narrows rows by search', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable status={status([node('node-a', 'active', { self: true }), node('node-b', 'standby')])} />,
    );
    expect(screen.getByTestId('ha-node-node-a')).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText('Search nodes, hosts, versions...'), 'node-b');
    await waitFor(() => expect(screen.queryByTestId('ha-node-node-a')).not.toBeInTheDocument());
    expect(screen.getByTestId('ha-node-node-b')).toBeInTheDocument();
  });

  it('narrows rows with the health facet', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable
        status={status([
          node('node-a', 'active', { self: true }),
          node('node-b', 'standby', { alive: false }),
          node('node-c', 'standby', { state: 'draining' }),
        ])}
      />,
    );
    await user.click(screen.getByRole('button', { name: /^Down/ }));
    await waitFor(() => expect(screen.queryByTestId('ha-node-node-a')).not.toBeInTheDocument());
    expect(screen.getByTestId('ha-node-node-b')).toBeInTheDocument();
    expect(screen.queryByTestId('ha-node-node-c')).not.toBeInTheDocument();
  });

  it('isolates nodes whose release differs from this node with the drift facet', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <ClusterInstancesTable
        status={status([
          node('node-a', 'active', { self: true }),
          node('node-b', 'standby', { build: '202609010000' }),
          node('node-c', 'standby'),
        ])}
      />,
    );
    await user.click(screen.getByRole('button', { name: /^Version drift/ }));
    await waitFor(() => expect(screen.queryByTestId('ha-node-node-a')).not.toBeInTheDocument());
    expect(screen.getByTestId('ha-node-node-b')).toBeInTheDocument();
    expect(screen.queryByTestId('ha-node-node-c')).not.toBeInTheDocument();
  });

  it('warns on every page when this node is a standby', async () => {
    server.use(
      http.get('*/api/health', () =>
        HttpResponse.json({
          success: true,
          data: { status: 'ok', instanceId: 'x', nodeId: 'node-b', role: 'standby' },
        }),
      ),
    );
    renderWithProviders(<StandbyBanner />);
    expect(await screen.findByText('This is a standby node')).toBeInTheDocument();
  });
});
