// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentInventoryCard } from './AgentInventoryCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const failure = 'agent refused the binary: checksum mismatch for mcharbor-agent-linux-arm64 after 3 attempts';

const inventory = {
  targetVersion: '2026.09.01',
  targetBuild: '202609271200',
  excludedEnvIds: [],
  agents: [
    {
      envId: 'env-1',
      name: 'edge-01',
      version: '2026.09.01',
      build: 'dev',
      os: 'linux',
      arch: 'amd64',
      outdated: true,
      updatable: true,
      installKind: 'native',
    },
    {
      envId: 'env-2',
      name: 'edge-02',
      version: '2026.09.01',
      build: '202609271200',
      os: 'linux',
      arch: 'arm64',
      outdated: false,
      updatable: true,
      installKind: 'docker',
    },
    {
      envId: 'env-3',
      name: 'edge-03',
      version: '3.5.3',
      build: '202601010000',
      os: 'linux',
      arch: 'arm64',
      outdated: true,
      updatable: true,
      progress: { status: 'failed', error: failure },
    },
  ],
};

describe('AgentInventoryCard', () => {
  const updates = vi.fn();

  beforeEach(() => {
    updates.mockReset();
    grantPermissions(['environments.view', 'environments.manage']);
    server.use(
      http.get('*/api/table-views/settings-agent-inventory', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
      http.get('*/api/updates/agents', () => HttpResponse.json({ success: true, data: inventory })),
      http.post('*/api/updates/agents/update', async ({ request }) => {
        updates(await request.json());
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
  });

  it('shows a dev build as-is and keeps a failed update behind the error icon', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentInventoryCard />);
    const devRow = await screen.findByTestId('agent-row-env-1');
    expect(within(devRow).getByText('dev')).toBeInTheDocument();
    const failedRow = screen.getByTestId('agent-row-env-3');
    expect(within(failedRow).queryByText(failure)).not.toBeInTheDocument();
    await user.click(within(failedRow).getByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(failure)).toBeInTheDocument();
  });

  it('narrows the rows with the status facet', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentInventoryCard />);
    await screen.findByTestId('agent-row-env-1');
    await user.click(screen.getByRole('button', { name: /^Failed/ }));
    await waitFor(() => expect(screen.queryByTestId('agent-row-env-1')).not.toBeInTheDocument());
    expect(screen.getByTestId('agent-row-env-3')).toBeInTheDocument();
    expect(screen.queryByTestId('agent-row-env-2')).not.toBeInTheDocument();
  });

  it('updates one agent only after confirmation', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentInventoryCard />);
    const row = await screen.findByTestId('agent-row-env-1');
    await user.click(within(row).getByRole('button', { name: 'Update edge-01' }));
    const dialog = await screen.findByRole('dialog');
    expect(updates).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Update' }));
    await waitFor(() => expect(updates).toHaveBeenCalledTimes(1));
    expect(updates).toHaveBeenCalledWith({ envIds: ['env-1'] });
  });

  it('renders the row update disabled, naming environments.manage, when it is missing', async () => {
    grantPermissions(['environments.view']);
    const user = userEvent.setup();
    renderWithProviders(<AgentInventoryCard />);
    const row = await screen.findByTestId('agent-row-env-1');
    const button = within(row).getByRole('button', { name: 'Update edge-01' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the environments.manage permission')).not.toHaveLength(0);
    expect(updates).not.toHaveBeenCalled();
  });
});
