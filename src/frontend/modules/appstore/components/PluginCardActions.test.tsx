// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginCardActions } from './PluginCardActions';
import type { PluginEntry } from '../types/plugins';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const plugin = {
  slug: 'sample-tool',
  name: 'Slack',
  version: '2026.09.02',
  installed: true,
} as PluginEntry & { installed: boolean };

function mount(installedEnabled = true) {
  return renderWithProviders(
    <PluginCardActions plugin={plugin} installedEnabled={installedEnabled} onInstall={vi.fn()} onUpdate={vi.fn()} />,
  );
}

describe('PluginCardActions reconcile', () => {
  const calls = vi.fn();

  beforeEach(() => {
    calls.mockReset();
    server.use(
      http.get('*/api/appstore/plugins/*', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
      http.post('*/api/appstore/plugins/sample-tool/reconcile', async ({ request }) => {
        calls(await request.json());
        return HttpResponse.json({
          success: true,
          data: { slug: 'sample-tool', version: '2026.09.02', action: 'kept' },
        });
      }),
    );
  });

  it('disables reconcile and names plugins.manage when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const button = screen.getByRole('button', { name: 'Reconcile' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the plugins.manage permission')).not.toHaveLength(0);
    expect(calls).not.toHaveBeenCalled();
  });

  it('sends one request pinning the catalog version and keeping the enabled flag', async () => {
    grantPermissions(['plugins.manage']);
    const user = userEvent.setup();
    mount(false);
    await user.click(screen.getByRole('button', { name: 'Reconcile' }));
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(1));
    expect(calls).toHaveBeenCalledWith({ version: '2026.09.02', enabled: false });
  });
});
