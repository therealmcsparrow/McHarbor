// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { McHarborUpdateCard } from './McHarborUpdateCard';
import { StorageLocationSetupGuide } from './StorageLocationSetupGuide';

const about = (native: boolean, sandboxed = false) =>
  http.get('*/api/about', () =>
    HttpResponse.json({ success: true, data: { version: '2026.10.01', native, sandboxed } }),
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

  it('offers Update now on a container install', async () => {
    server.use(about(false));
    renderWithProviders(<McHarborUpdateCard />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Check Now' }));
    expect(await screen.findByTestId('self-update-button')).toBeInTheDocument();
    expect(screen.queryByText(/Upgrade it with your package manager/)).not.toBeInTheDocument();
  });
});
