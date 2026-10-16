// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DemoModePanel } from './DemoModePanel';
import type { DemoStatus } from '@resources/types/demo';
import { DEMO_STATUS_SIZES } from '../test-fixtures';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

vi.mock('@resources/stores/mcharbor-restart', () => ({
  runWithMcHarborRestart: vi.fn(async (request: () => Promise<unknown>) => request()),
}));

const inactive: DemoStatus = {
  available: true,
  active: false,
  databaseExists: false,
  clustered: false,
  canEnter: true,
  ...DEMO_STATUS_SIZES,
};

const running: DemoStatus = {
  available: true,
  active: true,
  databaseExists: true,
  clustered: false,
  canEnter: false,
  enteredAt: '2026-09-27T10:00:00Z',
  ...DEMO_STATUS_SIZES,
  config: { ...DEMO_STATUS_SIZES.defaults, environments: 6 },
  counts: {
    environments: 6,
    containers: 1234,
    stacks: 151,
    virtualMachines: 121,
    systemContainers: 81,
    clusters: 3,
    pods: 412,
    swarmClusters: 2,
    swarmNodes: 9,
    swarmServices: 61,
    historyDays: 90,
  },
};

describe('DemoModePanel', () => {
  const posts = vi.fn();

  function serve(status: DemoStatus) {
    server.use(
      http.get('*/api/demo/status', () => HttpResponse.json({ success: true, data: status })),
      http.post('*/api/demo/:action', ({ params }) => {
        posts(params.action);
        return HttpResponse.json({ success: true, data: { restartScheduled: false, restartRequired: false } });
      }),
    );
  }

  beforeEach(() => {
    posts.mockReset();
    grantPermissions(['system.demo']);
  });

  it('opens a confirmation before entering, and sends nothing until it is confirmed', async () => {
    serve(inactive);
    const user = userEvent.setup();
    renderWithProviders(<DemoModePanel />);
    await user.click(await screen.findByRole('button', { name: 'Enter demo mode' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Enter demo mode?')).toBeInTheDocument();
    expect(posts).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(posts).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Reset demo data' })).not.toBeInTheDocument();
  });

  it('renders Enter disabled, naming system.demo, without the permission', async () => {
    grantPermissions([]);
    serve(inactive);
    const user = userEvent.setup();
    renderWithProviders(<DemoModePanel />);
    const button = await screen.findByRole('button', { name: 'Enter demo mode' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the system.demo permission')).not.toHaveLength(0);
  });

  it('shows the live estate with Exit and Reset while active', async () => {
    serve(running);
    renderWithProviders(<DemoModePanel />);
    expect(await screen.findByText('What the demo estate contains right now')).toBeInTheDocument();
    expect(screen.getByText('412')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Exit demo mode' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Reset demo data' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Apply new sizes' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Enter demo mode' })).not.toBeInTheDocument();
  });

  it('opens Apply new sizes prefilled with the running sizes, and sends nothing until confirmed', async () => {
    serve(running);
    const user = userEvent.setup();
    renderWithProviders(<DemoModePanel />);
    await user.click(await screen.findByRole('button', { name: 'Apply new sizes' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Apply new demo sizes?')).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Environments')).toHaveValue(6);
    expect(posts).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(posts).not.toHaveBeenCalled();
  });

  it('renders Apply new sizes disabled, naming system.demo, without the permission', async () => {
    grantPermissions([]);
    serve(running);
    const user = userEvent.setup();
    renderWithProviders(<DemoModePanel />);
    const button = await screen.findByRole('button', { name: 'Apply new sizes' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the system.demo permission')).not.toHaveLength(0);
  });

  it('keeps Enter disabled on a clustered instance and says why', async () => {
    serve({ ...inactive, clustered: true, canEnter: false });
    renderWithProviders(<DemoModePanel />);
    expect(await screen.findByText('Unavailable on a clustered instance')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Enter demo mode' })).toBeDisabled();
  });
});
