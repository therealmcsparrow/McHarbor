// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import NotificationsPage from './NotificationsPage';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const item = {
  id: 'n1',
  level: 'error',
  title: 'Backup failed',
  message: 'volume db_data unreadable',
  createdAt: '2026-09-27T10:00:00Z',
  read: false,
};

describe('Notifications page', () => {
  let requests: URLSearchParams[];

  beforeEach(() => {
    requests = [];
    grantPermissions(['in_app_notifications.view']);
    server.use(
      http.get('*/api/in-app-notifications', ({ request }) => {
        requests.push(new URL(request.url).searchParams);
        return HttpResponse.json({
          success: true,
          data: { items: [item], total: 1, page: 1, per_page: 25, total_pages: 1 },
        });
      }),
      http.get('*/api/in-app-notifications/unread-count', () =>
        HttpResponse.json({ success: true, data: { count: 1 } }),
      ),
      http.get('*/api/in-app-notifications/counts', () =>
        HttpResponse.json({ success: true, data: { total: 5, unread: 2, byLevel: { error: 1, info: 4 } } }),
      ),
    );
  });

  it('searches and filters on the server, so every page is covered', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <MemoryRouter>
        <NotificationsPage />
      </MemoryRouter>,
    );
    expect(await screen.findByText('Backup failed')).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText('Search title and message'), 'db_data');
    await waitFor(() => expect(requests.at(-1)?.get('q')).toBe('db_data'));
    expect(requests.at(-1)?.get('mode')).toBe('contains');

    await user.click(screen.getByRole('button', { name: /Unread/ }));
    await waitFor(() => expect(requests.at(-1)?.get('status')).toBe('unread'));
    expect(requests.at(-1)?.get('page')).toBe('1');
    expect(screen.getByRole('button', { name: /Read\s*3/ })).toBeInTheDocument();
  });
});
