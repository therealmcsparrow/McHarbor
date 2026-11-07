// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { McHarborUpdateCard } from './McHarborUpdateCard';
import { StorageLocationSetupGuide } from './StorageLocationSetupGuide';

const about = (native: boolean, sandboxed = false, selfUpdate = false) =>
  http.get('*/api/about', () =>
    HttpResponse.json({ success: true, data: { version: '2026.10.01', native, sandboxed, selfUpdate } }),
  );

describe('native install wording', () => {
  beforeEach(() => {
    grantPermissions('*');
    server.use(
      http.get('*/api/plugins/loader/storage-adapters', () => HttpResponse.json({ success: true, data: {} })),
      http.get('*/api/updates/check', () =>
        HttpResponse.json({
          success: true,
          data: { currentVersion: '2026.10.01', latestVersion: '2026.10.02', updateAvailable: true },
        }),
      ),
    );
  });

  it('tells a sandboxed native install to open the path to the systemd service, not to mount it into a container', async () => {
    server.use(about(true, true));
    renderWithProviders(<StorageLocationSetupGuide locationType="local" />);
    expect(await screen.findByText(/needs a ReadWritePaths= drop-in for mcharbor\.service/)).toBeInTheDocument();
    expect(screen.queryByText(/into the McHarbor container/)).not.toBeInTheDocument();
  });

  it('tells a native install running as root that any local path works', async () => {
    server.use(about(true));
    renderWithProviders(<StorageLocationSetupGuide locationType="local" />);
    expect(await screen.findByText(/McHarbor runs as root here, so any local path works/)).toBeInTheDocument();
    expect(screen.queryByText(/ReadWritePaths=/)).not.toBeInTheDocument();
  });

  it('keeps the container instructions for a container install', async () => {
    server.use(about(false));
    renderWithProviders(<StorageLocationSetupGuide locationType="local" />);
    expect(await screen.findByText(/into the McHarbor container/)).toBeInTheDocument();
  });

  it('offers no container self-update on a native install and says how to upgrade instead', async () => {
    server.use(about(true));
    renderWithProviders(<McHarborUpdateCard />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Check Now' }));
    expect(await screen.findByText(/Upgrade it with your package manager/)).toBeInTheDocument();
    expect(screen.queryByTestId('self-update-button')).not.toBeInTheDocument();
  });

  it('offers Update now on a native install that can update itself, and describes the package install', async () => {
    server.use(about(true, false, true));
    renderWithProviders(<McHarborUpdateCard />);
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Check Now' }));
    await user.click(await screen.findByTestId('self-update-button'));
    expect(await screen.findByText(/download this host's \.deb, \.rpm or tarball/)).toBeInTheDocument();
    expect(screen.queryByText(/pull the new image/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Upgrade it with your package manager/)).not.toBeInTheDocument();
  });

  it('names the target version when a native host updates itself, because an unnamed target is refused', async () => {
    const bodies: unknown[] = [];
    server.use(
      about(true, false, true),
      http.get('*/api/health', () => HttpResponse.json({ success: true, data: { status: 'ok', instanceId: 'a' } })),
      http.post('*/api/versions/self-update', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: { message: 'scheduled' } });
      }),
    );
    renderWithProviders(<McHarborUpdateCard />);
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Check Now' }));
    await user.click(await screen.findByTestId('self-update-button'));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Update now' }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({ version: '2026.10.02' });
  });

  it('keeps a sandboxed native install on the manual upgrade path', async () => {
    server.use(about(true, true, false));
    renderWithProviders(<McHarborUpdateCard />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Check Now' }));
    expect(await screen.findByText(/runs as the sandboxed mcharbor account/)).toBeInTheDocument();
    expect(screen.queryByTestId('self-update-button')).not.toBeInTheDocument();
  });

  it('offers Update now on a container install', async () => {
    server.use(about(false));
    renderWithProviders(<McHarborUpdateCard />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Check Now' }));
    expect(await screen.findByTestId('self-update-button')).toBeInTheDocument();
    expect(screen.queryByText(/Upgrade it with your package manager/)).not.toBeInTheDocument();
  });
});
