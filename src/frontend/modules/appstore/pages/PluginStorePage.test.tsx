// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import PluginStorePage from './PluginStorePage';
import type { InstalledPlugin, PluginEntry } from '../types/plugins';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const entry: PluginEntry = {
  slug: 'alpha',
  name: 'Alpha',
  version: '2026.11.02-rc.1',
  author: '',
  license: '',
  homepage: '',
  description: 'Alpha plugin',
  category: 'addons',
  download_url: '',
  versions: [{ version: '2026.11.02-rc.1', download_url: '' }],
};

const installedRow = {
  id: 'alpha',
  slug: 'alpha',
  version: '2026.11.01',
  enabled: true,
  signed: true,
} as InstalledPlugin;

const puts = vi.fn();
const reads = vi.fn();
let stored = { pluginPrereleases: false };

beforeEach(() => {
  puts.mockReset();
  reads.mockReset();
  stored = { pluginPrereleases: false };
  server.use(
    http.get('*/api/appstore/plugins/catalog', () => HttpResponse.json({ success: true, data: [entry] })),
    http.get('*/api/appstore/plugins/installed', () => HttpResponse.json({ success: true, data: [installedRow] })),
    http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/updates/channels', () => {
      reads();
      return HttpResponse.json({ success: true, data: stored });
    }),
    http.put('*/api/updates/channels', async ({ request }) => {
      const body = (await request.json()) as { pluginPrereleases: boolean };
      puts(body);
      stored = { ...stored, ...body };
      return HttpResponse.json({ success: true, data: stored });
    }),
  );
});

describe('Plugin Store header switch', () => {
  it('sends only the changed field when an admin turns it on', async () => {
    grantPermissions(['updates.view', 'updates.manage', 'plugins.view']);
    const user = userEvent.setup();
    renderWithProviders(<PluginStorePage />);
    const toggle = await screen.findByRole('switch', { name: 'Offer plugin prereleases' });
    await waitFor(() => expect(toggle).not.toHaveAttribute('aria-busy', 'true'));
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(toggle).toHaveAccessibleDescription(/test builds that are not final/);
    await user.click(toggle);
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith({ pluginPrereleases: true });
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'));
  });

  it('shows the current state disabled and names updates.manage when only view is held', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    renderWithProviders(<PluginStorePage />);
    const toggle = await screen.findByRole('switch', { name: 'Offer plugin prereleases' });
    await waitFor(() => expect(toggle).toHaveAccessibleDescription(/Needs updates\.manage\./));
    expect(toggle).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByText('Needs updates.manage.')).toBeVisible();
    await user.click(toggle);
    expect(puts).not.toHaveBeenCalled();
    expect(toggle).toHaveAttribute('aria-checked', 'false');
  });

  it('says the setting is unavailable without updates.view and never asks for it', async () => {
    grantPermissions([]);
    renderWithProviders(<PluginStorePage />);
    const toggle = await screen.findByRole('switch', { name: 'Offer plugin prereleases' });
    expect(toggle).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByText('Setting unavailable. Needs updates.view.')).toBeVisible();
    expect(reads).not.toHaveBeenCalled();
    expect(screen.queryByText(/Could not load/)).not.toBeInTheDocument();
  });

  it('offers a retry beside a disabled switch when the read fails', async () => {
    grantPermissions(['updates.view', 'updates.manage']);
    server.use(
      http.get('*/api/updates/channels', () => HttpResponse.json({ success: false, error: 'down' }, { status: 500 })),
    );
    renderWithProviders(<PluginStorePage />);
    expect(await screen.findByText('Could not load the prerelease setting.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeEnabled();
    expect(screen.getByRole('switch', { name: 'Offer plugin prereleases' })).toHaveAttribute('aria-disabled', 'true');
  });
});

describe('Plugin card labels', () => {
  it('announces the prerelease as text, not colour', async () => {
    grantPermissions(['plugins.view']);
    renderWithProviders(<PluginStorePage />);
    expect(await screen.findAllByText('Prerelease update available')).not.toHaveLength(0);
    expect(screen.getByRole('article', { name: /Alpha.*prerelease update available/i })).toBeInTheDocument();
  });
});

describe('Plugin detail dialog', () => {
  it('follows the live catalog after the override is turned on, without reopening', async () => {
    grantPermissions(['plugins.view', 'plugins.manage', 'updates.view']);
    let overrideOn = false;
    const stable = { version: '2026.11.01', download_url: '' };
    server.use(
      http.get('*/api/appstore/plugins/catalog', () =>
        HttpResponse.json({
          success: true,
          data: [
            {
              ...entry,
              version: '2026.11.01',
              versions: overrideOn ? [{ version: '2026.11.02-rc.1', download_url: '' }, stable] : [stable],
            },
          ],
        }),
      ),
      http.put('*/api/appstore/plugins/:slug/channel', () => {
        overrideOn = true;
        return HttpResponse.json({ success: true, data: { slug: 'alpha', prereleases: true } });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<PluginStorePage />);
    await user.click(await screen.findByRole('button', { name: 'View details' }));
    const toggle = await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    await user.click(toggle);
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'));
    await waitFor(() => expect(overrideOn).toBe(true));
    // The refreshed catalog carries a prerelease, so the "none available" reason must go.
    await waitFor(() => expect(screen.queryByText('No prerelease available.')).not.toBeInTheDocument());
  });
});
