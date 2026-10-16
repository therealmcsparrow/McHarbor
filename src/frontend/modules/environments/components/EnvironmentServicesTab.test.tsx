// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EnvironmentServicesTab } from './EnvironmentServicesTab';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const failure =
  'result: exit-code; main process exited with status 1; status: bind() to 0.0.0.0:80 failed (98: Address already in use)';

const listing = {
  availability: {
    supported: true,
    minVersion: '2026.09.01',
    minBuild: '202609271515',
  },
  units: [
    {
      name: 'nginx.service',
      description: 'A high performance web server',
      loadState: 'loaded',
      activeState: 'failed',
      subState: 'failed',
      unitFileState: 'enabled',
      failureReason: failure,
    },
    {
      name: 'sshd.service',
      description: 'OpenSSH server',
      loadState: 'loaded',
      activeState: 'active',
      subState: 'running',
      unitFileState: 'enabled',
      mainPid: 812,
      protected: 'ssh',
    },
    {
      name: 'cron.service',
      description: 'Regular background program processing daemon',
      loadState: 'loaded',
      activeState: 'inactive',
      subState: 'dead',
      unitFileState: 'disabled',
    },
    {
      name: 'mcharbor-agent.service',
      description: 'McHarbor agent',
      loadState: 'loaded',
      activeState: 'active',
      subState: 'running',
      unitFileState: 'enabled',
      mainPid: 91,
      protected: 'agent',
      isAgent: true,
    },
  ],
};

describe('EnvironmentServicesTab', () => {
  const actions = vi.fn();

  beforeEach(() => {
    actions.mockReset();
    grantPermissions(['host_services.view', 'host_services.manage']);
    server.use(
      http.get('*/api/table-views/environment-host-services', () =>
        HttpResponse.json({
          success: true,
          data: { layout: null, filters: [] },
        }),
      ),
      http.get('*/api/host-services', () => HttpResponse.json({ success: true, data: listing })),
      http.post('*/api/host-services/:unit/:action', ({ params, request }) => {
        actions(params.unit, params.action, new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          data: { unit: params.unit, action: params.action },
        });
      }),
    );
  });

  it("keeps a failed unit's reason behind the error icon", async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentServicesTab envId="env-1" />);
    const row = await screen.findByTestId('host-service-nginx.service');
    expect(within(row).queryByText(failure)).not.toBeInTheDocument();
    await user.click(within(row).getByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(failure)).toBeInTheDocument();
  });

  it('narrows the rows with the failed facet', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentServicesTab envId="env-1" />);
    await screen.findByTestId('host-service-cron.service');
    await user.click(screen.getByRole('button', { name: /^Failed/ }));
    await waitFor(() => expect(screen.queryByTestId('host-service-cron.service')).not.toBeInTheDocument());
    expect(screen.getByTestId('host-service-nginx.service')).toBeInTheDocument();
  });

  it('disables stop and disable on a protected unit and says why', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentServicesTab envId="env-1" />);
    const row = await screen.findByTestId('host-service-sshd.service');
    const stop = within(row).getByRole('button', { name: 'Stop sshd.service' });
    expect(stop).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Disable sshd.service' })).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Restart sshd.service' })).toBeEnabled();
    await user.hover(stop.parentElement as HTMLElement);
    expect(await screen.findAllByText('Protected: stopping SSH could lock you out of the host.')).not.toHaveLength(0);
    expect(actions).not.toHaveBeenCalled();
  });

  it('starts a unit directly but stops one only after confirmation', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentServicesTab envId="env-1" />);
    const cron = await screen.findByTestId('host-service-cron.service');
    await user.click(within(cron).getByRole('button', { name: 'Start cron.service' }));
    await waitFor(() => expect(actions).toHaveBeenCalledWith('cron.service', 'start', 'env-1'));

    actions.mockReset();
    const agent = screen.getByTestId('host-service-mcharbor-agent.service');
    await user.click(
      within(agent).getByRole('button', {
        name: 'Restart mcharbor-agent.service',
      }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/drops the connection to this host/)).toBeInTheDocument();
    expect(actions).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Restart' }));
    await waitFor(() => expect(actions).toHaveBeenCalledWith('mcharbor-agent.service', 'restart', 'env-1'));
  });

  it('names the build an outdated agent needs instead of a table', async () => {
    server.use(
      http.get('*/api/host-services', () =>
        HttpResponse.json({
          success: true,
          data: {
            availability: {
              supported: false,
              reason: 'agent_outdated',
              agentVersion: '2026.09.01',
              agentBuild: '202609250000',
              minVersion: '2026.09.01',
              minBuild: '202609271515',
            },
            units: [],
          },
        }),
      ),
    );
    renderWithProviders(<EnvironmentServicesTab envId="env-1" />);
    expect(await screen.findByText('Update the agent to use Services')).toBeInTheDocument();
    expect(screen.getByText(/2026\.09\.01 build 202609271515 or newer/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });
});
