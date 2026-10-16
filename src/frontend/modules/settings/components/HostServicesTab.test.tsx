// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { HostServicesTab } from './HostServicesTab';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const listing = {
  availability: { supported: true, minVersion: '', minBuild: '' },
  units: [
    {
      name: 'mcharbor.service',
      description: 'McHarbor',
      loadState: 'loaded',
      activeState: 'active',
      subState: 'running',
      unitFileState: 'enabled',
      mainPid: 42,
      protected: 'mcharbor',
      restartsMcHarbor: true,
    },
    {
      name: 'docker.service',
      description: 'Docker Application Container Engine',
      loadState: 'loaded',
      activeState: 'active',
      subState: 'running',
      unitFileState: 'enabled',
      mainPid: 900,
      protected: 'self_runtime',
      restartsMcHarbor: true,
    },
    {
      name: 'cron.service',
      description: 'Regular background program processing daemon',
      loadState: 'loaded',
      activeState: 'inactive',
      subState: 'dead',
      unitFileState: 'disabled',
    },
  ],
};

describe('Settings → System → Services', () => {
  const lists = vi.fn();
  const actions = vi.fn();

  beforeEach(() => {
    lists.mockReset();
    actions.mockReset();
    grantPermissions(['host_self.view', 'host_self.manage']);
    server.use(
      http.get('*/api/table-views/settings-host-services', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
      http.get('*/api/host-services/self', ({ request }) => {
        lists(new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({ success: true, data: listing });
      }),
      http.post('*/api/host-services/self/:unit/:action', ({ params, request }) => {
        actions(params.unit, params.action, new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({ success: true, data: { unit: params.unit, action: params.action } });
      }),
    );
  });

  it("lists McHarbor's own host without naming an environment", async () => {
    renderWithProviders(<HostServicesTab />);
    expect(await screen.findByTestId('host-service-cron.service')).toBeInTheDocument();
    expect(lists).toHaveBeenCalledWith(null);
  });

  it("refuses to stop McHarbor's own unit and says why", async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostServicesTab />);
    const row = await screen.findByTestId('host-service-mcharbor.service');
    const stop = within(row).getByRole('button', { name: 'Stop mcharbor.service' });
    expect(stop).toBeDisabled();
    expect(within(row).getByRole('button', { name: 'Disable mcharbor.service' })).toBeDisabled();
    await user.hover(stop.parentElement as HTMLElement);
    expect(
      await screen.findAllByText('Protected: this runs McHarbor itself. It can be restarted, not stopped or disabled.'),
    ).not.toHaveLength(0);
    expect(actions).not.toHaveBeenCalled();
  });

  it('warns that restarting the runtime takes McHarbor down before it confirms', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostServicesTab />);
    const row = await screen.findByTestId('host-service-docker.service');
    await user.click(within(row).getByRole('button', { name: 'Restart docker.service' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/takes McHarbor down with it/)).toBeInTheDocument();
    expect(actions).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Restart' }));
    await waitFor(() => expect(actions).toHaveBeenCalledWith('docker.service', 'restart', null));
  });

  it('shows why the host cannot be reached instead of an empty table', async () => {
    const reason = 'McHarbor runs in a container but cannot find itself on the Docker daemon it is connected to.';
    server.use(
      http.get('*/api/host-services/self', () =>
        HttpResponse.json({
          success: true,
          data: {
            availability: {
              supported: false,
              reason: 'self_unreachable',
              detail: reason,
              minVersion: '',
              minBuild: '',
            },
            units: [],
          },
        }),
      ),
    );
    renderWithProviders(<HostServicesTab />);
    expect(await screen.findByText('McHarbor cannot reach the machine it is installed on')).toBeInTheDocument();
    expect(screen.getByText(reason)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('leaves the reload control visible but disabled without the manage key', async () => {
    grantPermissions(['host_self.view']);
    renderWithProviders(<HostServicesTab />);
    await screen.findByTestId('host-service-cron.service');
    expect(screen.getByRole('button', { name: /Reload systemd/ })).toBeDisabled();
    expect(
      within(screen.getByTestId('host-service-cron.service')).getByRole('button', {
        name: 'Start cron.service',
      }),
    ).toBeDisabled();
  });
});
