// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import PluginStorePage from './PluginStorePage';
import type { InstalledPlugin, PluginEntry } from '../types/plugins';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const SLUGS = ['alpha', 'beta', 'gamma'];

function entry(slug: string): PluginEntry {
  return {
    slug,
    name: `Plugin ${slug}`,
    version: '2026.09.02',
    author: '',
    license: '',
    homepage: '',
    description: '',
    category: 'addons',
    download_url: '',
    versions: [{ version: '2026.09.02', download_url: '' }],
  };
}

function installedRow(slug: string): InstalledPlugin {
  return { id: slug, slug, version: '2026.09.01', enabled: true, signed: true } as InstalledPlugin;
}

const sse = (events: object[]) =>
  new HttpResponse(events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join(''), {
    headers: { 'Content-Type': 'text/event-stream' },
  });

const progress = (message: string) => ({ phase: 'extract', step: 1, total: 100, status: 'progress', message });
const done = { phase: 'done', step: 100, total: 100, status: 'done', result: {} };

let requested: string[] = [];
let inFlight = 0;
let maxInFlight = 0;

function stubStore(failing: string[] = []) {
  server.use(
    http.get('*/api/appstore/plugins/catalog', () => HttpResponse.json({ success: true, data: SLUGS.map(entry) })),
    http.get('*/api/appstore/plugins/installed', () =>
      HttpResponse.json({ success: true, data: SLUGS.map(installedRow) }),
    ),
    http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
    http.post('*/api/appstore/plugins/:slug/update/stream', async ({ params }) => {
      const slug = String(params.slug);
      requested.push(slug);
      inFlight += 1;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((resolve) => setTimeout(resolve, 20));
      inFlight -= 1;
      if (failing.includes(slug)) {
        return sse([
          progress('Downloading'),
          {
            phase: 'error',
            step: 0,
            total: 100,
            status: 'error',
            error_code: 'err.x',
            error_text: `migration 004 failed for ${slug}`,
          },
        ]);
      }
      return sse([progress('Downloading'), done]);
    }),
  );
}

async function openUpdateAll() {
  const user = userEvent.setup();
  renderWithProviders(<PluginStorePage />);
  const button = await screen.findByRole('button', { name: /Update all 3 plugins/ });
  return { user, button };
}

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
  requested = [];
  inFlight = 0;
  maxInFlight = 0;
  grantPermissions(['plugins.manage']);
});

describe('Plugin Store update all', () => {
  it('opens one dialog listing every plugin and updates them one at a time', async () => {
    stubStore();
    const { user, button } = await openUpdateAll();
    await user.click(button);

    const dialog = await screen.findByRole('dialog');
    for (const slug of SLUGS) expect(within(dialog).getByText(`Plugin ${slug}`)).toBeInTheDocument();
    await within(dialog).findByText('Update complete');
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    expect(within(dialog).getAllByText('Done')).toHaveLength(3);
    expect(requested).toEqual(SLUGS);
    expect(maxInFlight).toBe(1);
  });

  it('shows a failing plugin message on its row and carries on with the next plugin', async () => {
    stubStore(['beta']);
    const { user, button } = await openUpdateAll();
    await user.click(button);

    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText('Update finished with 1 failures');
    expect(requested).toEqual(SLUGS);
    expect(within(dialog).getAllByText('Done')).toHaveLength(2);
    expect(within(dialog).getAllByText('Failed')).toHaveLength(1);
    expect(screen.getAllByRole('dialog')).toHaveLength(1);

    await user.click(within(dialog).getByRole('button', { name: 'View the error' }));
    const details = await screen.findByRole('dialog', { name: 'Error details' });
    expect(within(details).getByText('migration 004 failed for beta')).toBeInTheDocument();
  });

  it('sends one request per plugin on a double click', async () => {
    stubStore();
    const { user, button } = await openUpdateAll();
    await user.dblClick(button);

    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText('Update complete');
    await waitFor(() => expect(requested).toEqual(SLUGS));
  });

  it('disables update all without plugins.manage and starts nothing', async () => {
    grantPermissions([]);
    stubStore();
    const { user, button } = await openUpdateAll();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(requested).toEqual([]);
  });
});
