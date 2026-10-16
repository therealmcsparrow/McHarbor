// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { EnvironmentHostUpdatesTab } from '@resources/components/host/EnvironmentHostUpdatesTab';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostUpdatesTab } from './HostUpdatesTab';

const state = {
  schedule: {
    enabled: false,
    time: '03:00',
    days: ['mon'],
    withNewPackages: false,
    skipUnavailable: false,
  },
  available: true,
  count: 1,
  manager: 'apt',
  updates: ['openssl/stable 3.0.15'],
};

const info = { prettyName: 'Debian GNU/Linux 12', manager: 'apt', rebootRequired: false };

describe('Settings host OS updates', () => {
  let urls: string[] = [];

  beforeEach(() => {
    urls = [];
    grantPermissions(['host_self.view', 'host_self.manage']);
    useEnvironmentStore.setState({ currentId: 'env-selected' });
    const record = (request: Request) => {
      const url = new URL(request.url);
      urls.push(`${request.method} ${url.pathname}${url.search}`);
    };
    server.use(
      http.get('*/api/system/self/os-info', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: info });
      }),
      http.get('*/api/system/self/os-updates/state', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: state });
      }),
      http.get('*/api/system/self/os-updates/check', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: {} });
      }),
      http.post('*/api/system/self/os-updates/apply', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: {} });
      }),
      http.get('*/api/system/os-info', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: info });
      }),
      http.get('*/api/system/os-updates/state', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: state });
      }),
      http.get('*/api/system/os-updates/check', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: {} });
      }),
      http.post('*/api/system/os-updates/apply', ({ request }) => {
        record(request);
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
  });

  it('acts on the McHarbor host and never sends the selected environment', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostUpdatesTab />);

    expect(await screen.findByText('Debian GNU/Linux 12')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Check now' }));
    await user.click(await screen.findByRole('button', { name: 'Update host' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Update host' }));

    await waitFor(() => expect(urls).toContain('POST /api/system/self/os-updates/apply'));
    expect(urls).toContain('GET /api/system/self/os-info');
    expect(urls).toContain('GET /api/system/self/os-updates/state');
    expect(urls).toContain('GET /api/system/self/os-updates/check');
    for (const url of urls) {
      expect(url).toContain('/api/system/self/');
      expect(url).not.toContain('env-selected');
      expect(url).not.toContain('env=');
    }
  });

  it('renders the same panel as the environment tab, with its schedule options', async () => {
    const first = renderWithProviders(<HostUpdatesTab />);
    expect(await screen.findByText('Host OS updates')).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Skip unavailable packages' })).toBeInTheDocument();
    first.unmount();

    renderWithProviders(<EnvironmentHostUpdatesTab envId="env-1" />);
    expect(await screen.findByText('Host OS updates')).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Skip unavailable packages' })).toBeInTheDocument();
    await waitFor(() => expect(urls).toContain('GET /api/system/os-updates/state?env=env-1'));
  });
});
