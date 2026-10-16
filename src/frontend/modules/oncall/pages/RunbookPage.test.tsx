// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter, Route, Routes } from 'react-router';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CACHED_AT_HEADER, isReadCacheable } from '@core/pwa/cachePolicy';
import { ONCALL_MEDIA_QUERY, resolveLayout } from '@resources/hooks/useLayoutMode';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import RunbookPage from './RunbookPage';

const resolved = {
  runbooks: [
    {
      id: 'rb-1',
      name: 'Restart loop',
      description: 'What to do when a container will not stay up',
      attachKind: 'alert_condition',
      attachSourceType: 'container',
      attachConditionKey: 'restarts',
      enabled: true,
      createdAt: '2026-09-26T03:00:00Z',
      updatedAt: '2026-09-26T03:00:00Z',
      steps: [
        {
          id: 'st-1',
          runbookId: 'rb-1',
          position: 1,
          kind: 'instruction',
          title: 'Check the dashboard',
          required: true,
        },
        {
          id: 'st-2',
          runbookId: 'rb-1',
          position: 2,
          kind: 'action',
          title: 'Restart the container',
          actionKey: 'container.restart',
          required: true,
          requiredPermissions: ['containers.manage'],
        },
      ],
    },
  ],
  openRuns: [],
};

function answer(cachedAt?: string) {
  return HttpResponse.json(
    { success: true, data: resolved },
    { headers: cachedAt ? { [CACHED_AT_HEADER]: cachedAt } : {} },
  );
}

function mount() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/oncall/runbooks/rb-1?env=env-1&container=web']}>
      <Routes>
        <Route path="/oncall/runbooks/:runbookId" element={<RunbookPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('On-call runbook page', () => {
  it('reads the runbook and its steps in one request the offline cache can serve', async () => {
    grantPermissions(['runbooks.view', 'runbooks.run']);
    server.use(http.get('*/api/runbooks/for', () => answer()));
    mount();

    expect(await screen.findByRole('heading', { name: 'Restart loop' })).toBeInTheDocument();
    expect(screen.getByText('Check the dashboard')).toBeInTheDocument();
    expect(screen.getByText('Restart the container')).toBeInTheDocument();
    expect(isReadCacheable('GET', new URL('https://host/api/runbooks/for?env=env-1'), 'https://host')).toBe(true);
  });

  it('marks a runbook served from the offline cache with the time it was fetched', async () => {
    grantPermissions(['runbooks.view']);
    server.use(http.get('*/api/runbooks/for', () => answer('2026-09-26T02:00:00.000Z')));
    mount();

    expect(await screen.findByText(/^Showing saved data from .+ It may be out of date\.$/)).toBeInTheDocument();
  });

  it('refuses to start a run while offline rather than queueing one', async () => {
    const calls = vi.fn();
    grantPermissions(['runbooks.view', 'runbooks.run']);
    server.use(
      http.get('*/api/runbooks/for', () => answer('2026-09-26T02:00:00.000Z')),
      http.post('*/api/runbooks/runs', () => {
        calls();
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
    const user = userEvent.setup();
    mount();

    expect(
      await screen.findByText(/You are offline\. You can read this runbook, but a step cannot be recorded/),
    ).toBeInTheDocument();
    const start = await screen.findByRole('button', { name: 'Start the run' });
    expect(start).toBeDisabled();
    await user.click(start);
    await waitFor(() => expect(calls).not.toHaveBeenCalled());
  });

  it('names the permission instead of calling the API when it is missing', async () => {
    grantPermissions([]);
    mount();
    expect(await screen.findByText('You need the runbooks.view permission to see this.')).toBeInTheDocument();
  });

  it('renders one column of full-width targets at the on-call breakpoint', async () => {
    grantPermissions(['runbooks.view', 'runbooks.run']);
    server.use(http.get('*/api/runbooks/for', () => answer()));
    expect(ONCALL_MEDIA_QUERY).toBe('(max-width: 639px)');
    expect(resolveLayout('auto', true)).toBe('oncall');
    mount();

    const start = await screen.findByRole('button', { name: 'Start the run' });
    expect(start.className).toContain('w-full');
    expect(start.className).toContain('min-h-12');
  });
});
