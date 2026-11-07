// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginDetailDialog } from './PluginDetailDialog';
import type { PluginEntry } from '../types/plugins';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const plugin: PluginEntry = {
  slug: 'alpha',
  name: 'Alpha',
  version: '2026.11.01',
  author: '',
  license: '',
  homepage: '',
  description: 'Alpha plugin',
  category: 'addons',
  download_url: '',
  versions: [
    { version: '2026.11.01', download_url: '' },
    { version: '2026.11.02-rc.1', download_url: '' },
  ],
};

const puts = vi.fn();
let globalOn = false;

function mount(props: { installedVersion?: string; entry?: PluginEntry } = {}) {
  return renderWithProviders(
    <PluginDetailDialog
      plugin={props.entry ?? plugin}
      open
      installed={props.installedVersion !== undefined}
      installedVersion={props.installedVersion}
      onOpenChange={vi.fn()}
      onInstall={vi.fn()}
    />,
  );
}

beforeEach(() => {
  puts.mockReset();
  globalOn = false;
  server.use(
    http.get('*/api/updates/channels', () =>
      HttpResponse.json({ success: true, data: { pluginPrereleases: globalOn } }),
    ),
    http.put('*/api/appstore/plugins/:slug/channel', async ({ request, params }) => {
      const body = (await request.json()) as { prereleases: boolean | null };
      puts(String(params.slug), body);
      return HttpResponse.json({ success: true, data: { slug: params.slug, prereleases: body.prereleases } });
    }),
    http.get('*/api/appstore/plugins/*', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
  );
});

describe('PluginDetailDialog prerelease override', () => {
  it('saves the override for this plugin with plugins.manage', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    const user = userEvent.setup();
    mount();
    const toggle = await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    expect(toggle).toHaveAccessibleDescription(/even when the Plugin Store switch is off/);
    await user.click(toggle);
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith('alpha', { prereleases: true });
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'));
  });

  it('is disabled and names plugins.manage when the key is missing', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    mount();
    const toggle = await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    expect(toggle).toHaveAttribute('aria-disabled', 'true');
    expect(toggle).toHaveAccessibleDescription(/Needs plugins\.manage\./);
    await user.click(toggle);
    expect(puts).not.toHaveBeenCalled();
  });

  it('shows on and disabled when the Store switch is on, pointing at the Plugin Store', async () => {
    globalOn = true;
    grantPermissions(['plugins.manage', 'updates.view']);
    const user = userEvent.setup();
    mount();
    const toggle = await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'));
    expect(toggle).toHaveAttribute('aria-disabled', 'true');
    expect(toggle).toHaveAccessibleDescription(/On for every plugin in the Plugin Store\./);
    await user.click(toggle);
    expect(puts).not.toHaveBeenCalled();
  });

  it('labels a prerelease version with the word, not a colour', async () => {
    grantPermissions(['plugins.manage']);
    mount();
    const item = await screen.findByText('v2026.11.02-rc.1', { selector: 'li span' });
    expect(item.closest('li')).toHaveTextContent('Prerelease');
    expect(screen.getByText('v2026.11.01', { selector: 'li span' }).closest('li')).not.toHaveTextContent('Prerelease');
  });

  it('says a prerelease stays installed when offers are off for the plugin', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    mount({ installedVersion: '2026.11.02-rc.1' });
    expect(
      await screen.findByText('Prerelease offers are off for this plugin. The next stable will be offered.'),
    ).toBeVisible();
    expect(screen.getByText('Installed: prerelease')).toBeVisible();
  });

  it('says so when no prerelease is available and the setting is on', async () => {
    globalOn = true;
    grantPermissions(['plugins.manage', 'updates.view']);
    mount({ entry: { ...plugin, versions: [{ version: '2026.11.01', download_url: '' }] } });
    expect(await screen.findByText('No prerelease available.')).toBeVisible();
  });
});

describe('PluginDetailDialog stored override', () => {
  it('reads the override the catalog entry carries, so the row is right after a reload', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    mount({ entry: { ...plugin, prereleaseOverride: true } });
    const toggle = await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    expect(toggle).toBeChecked();
  });
});

describe('PluginDetailDialog return to stable', () => {
  it('offers the return only when the installed version is a prerelease, and opens the dialog', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    server.use(
      http.get('*/api/appstore/plugins/:slug/return/check', () =>
        HttpResponse.json({ success: true, data: { ok: true, outcome: 'clear', target_version: '2026.11.01' } }),
      ),
    );
    const user = userEvent.setup();
    mount({ installedVersion: '2026.11.02-rc.1' });
    await user.click(await screen.findByRole('button', { name: 'Return to stable' }));
    expect(await screen.findByRole('dialog', { name: 'Return Alpha to stable?' })).toBeVisible();
  });

  it('shows no return action for a stable install or an uninstalled plugin', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    const first = mount({ installedVersion: '2026.11.01' });
    await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    expect(screen.queryByRole('button', { name: 'Return to stable' })).not.toBeInTheDocument();
    first.unmount();
    mount();
    await screen.findByRole('switch', { name: 'Offer prereleases for this plugin' });
    expect(screen.queryByRole('button', { name: 'Return to stable' })).not.toBeInTheDocument();
  });

  it('on a prerelease host shows Return to stable aria-disabled, focusable, with its reason visible and described', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    server.use(
      http.get('*/api/versions', () =>
        HttpResponse.json({
          success: true,
          data: { mcharbor: { name: 'McHarbor', version: '2026.11.02-rc.1' }, agents: [] },
        }),
      ),
    );
    const user = userEvent.setup();
    mount({ installedVersion: '2026.11.02-rc.1' });
    const button = await screen.findByRole('button', { name: 'Return to stable' });
    await waitFor(() => expect(button).toHaveAttribute('aria-disabled', 'true'));
    expect(button).not.toBeDisabled();
    expect(button).toHaveAccessibleDescription(/Plugins follow this prerelease host/);
    expect(screen.getByText(/Plugins follow this prerelease host/)).toBeVisible();
    await user.click(button);
    expect(screen.queryByRole('dialog', { name: 'Return Alpha to stable?' })).not.toBeInTheDocument();
  });

  it('on a stable host keeps Return to stable enabled with no reason', async () => {
    grantPermissions(['plugins.manage', 'updates.view']);
    server.use(
      http.get('*/api/versions', () =>
        HttpResponse.json({ success: true, data: { mcharbor: { name: 'McHarbor', version: '2026.11.01' }, agents: [] } }),
      ),
    );
    mount({ installedVersion: '2026.11.02-rc.1' });
    const button = await screen.findByRole('button', { name: 'Return to stable' });
    await waitFor(() => expect(screen.queryByText(/Plugins follow this prerelease host/)).not.toBeInTheDocument());
    expect(button).not.toHaveAttribute('aria-disabled', 'true');
  });
});
