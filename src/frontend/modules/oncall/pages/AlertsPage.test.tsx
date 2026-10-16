// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { CACHED_AT_HEADER } from '@core/pwa/cachePolicy';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { countStates } from '../hooks/useFleet';
import { mergeInbox } from '../hooks/useInbox';
import { base64UrlToBytes } from '../hooks/usePushDevice';
import { filterLines } from './LogsPage';
import AlertsPage from './AlertsPage';

const longMessage = `Container web exited with code 137. ${'The kernel OOM killer reclaimed memory. '.repeat(6)}END-OF-MESSAGE`;

function page(items: unknown[], cachedAt?: string) {
  return HttpResponse.json(
    { success: true, data: { items, total: items.length, page: 1, per_page: 50, total_pages: 1 } },
    { headers: cachedAt ? { [CACHED_AT_HEADER]: cachedAt } : {} },
  );
}

const firing = {
  id: 'ev1',
  alertId: 'a1',
  alertName: 'web down',
  sourceType: 'container',
  conditionKey: 'state',
  environmentId: 'env-1',
  environmentName: 'prod',
  subjectId: 'abc123',
  subjectName: 'web',
  state: 'firing',
  severity: 'critical',
  value: 0,
  message: longMessage,
  deliveries: [],
  createdAt: '2026-09-26T03:00:00Z',
};

function mount() {
  return renderWithProviders(
    <MemoryRouter>
      <AlertsPage />
    </MemoryRouter>,
  );
}

describe('On-call alert inbox', () => {
  it('links a container alert to its container and shows the whole message on demand', async () => {
    grantPermissions(['alerts.view']);
    server.use(http.get('*/api/alerts/events', () => page([firing])));
    const user = userEvent.setup();
    mount();
    const row = (await screen.findByText('web down')).closest('li') as HTMLElement;
    expect(within(row).getByRole('link', { name: 'Open container' })).toHaveAttribute(
      'href',
      '/oncall/containers/env-1/abc123',
    );
    expect(within(row).queryByText(/END-OF-MESSAGE/)).not.toBeInTheDocument();
    await user.click(within(row).getByRole('button', { name: 'Show the full message' }));
    expect(within(row).getByText(longMessage)).toBeInTheDocument();
  });

  it('marks data served from the offline cache with the time it was fetched', async () => {
    grantPermissions(['alerts.view']);
    server.use(http.get('*/api/alerts/events', () => page([firing], '2026-09-26T02:00:00.000Z')));
    mount();
    expect(await screen.findByText(/^Showing saved data from .+ It may be out of date\.$/)).toBeInTheDocument();
  });

  it('never shows a live-looking inbox without a staleness marker when the data is fresh', async () => {
    grantPermissions(['alerts.view']);
    server.use(http.get('*/api/alerts/events', () => page([firing])));
    mount();
    await screen.findByText('web down');
    expect(screen.queryByText(/saved data|Offline/)).not.toBeInTheDocument();
  });

  it('names the permission instead of calling the API when it is missing', async () => {
    grantPermissions([]);
    mount();
    expect(await screen.findByText('You need the alerts.view permission to see this.')).toBeInTheDocument();
  });
});

describe('On-call pure logic', () => {
  it('merges alerts and notices newest first', () => {
    const merged = mergeInbox(
      [{ ...firing, createdAt: '2026-09-26T01:00:00Z' } as never],
      [
        {
          id: 'n1',
          level: 'error',
          title: 'backup failed',
          message: 'x',
          createdAt: '2026-09-26T02:00:00Z',
          read: false,
        },
      ],
    );
    expect(merged.map((item) => item.id)).toEqual(['notice-n1', 'alert-ev1']);
    expect(merged[0]?.severity).toBe('error');
  });

  it('counts container states for the fleet glance', () => {
    const rows = ['running', 'running', 'exited', 'dead', 'restarting', 'paused'].map((State, i) => ({
      Id: String(i),
      State,
    }));
    expect(countStates(rows as never)).toEqual({ running: 2, stopped: 2, restarting: 1, other: 1 });
  });

  it('decodes a VAPID key to the 65 raw bytes the browser needs', () => {
    const raw = new Uint8Array(65).map((_, i) => (i * 7) % 256);
    const encoded = btoa(String.fromCharCode(...raw))
      .replace(/\+/g, '-')
      .replace(/[/]/g, '_')
      .replace(/=+$/, '');
    expect(Array.from(base64UrlToBytes(encoded))).toEqual(Array.from(raw));
  });

  it('searches the log case-insensitively', () => {
    expect(filterLines('a Error x\nfine\n\nERROR y', 'error')).toEqual(['a Error x', 'ERROR y']);
    expect(filterLines('one\ntwo\n', '')).toEqual(['one', 'two']);
  });
});
