// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import RunbooksPage from './RunbooksPage';
import RunbookEditRedirect from './RunbookEditRedirect';
import type { Runbook } from '@resources/types/runbooks';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const runbook: Runbook = {
  id: 'rb-1',
  name: 'Restart loop',
  description: 'When a container keeps restarting',
  attachKind: 'alert_condition',
  attachSourceType: 'container',
  attachConditionKey: 'restarts',
  enabled: true,
  createdAt: '2026-09-27T03:00:00Z',
  updatedAt: '2026-09-27T03:00:00Z',
  steps: [{ id: 'st-1', runbookId: 'rb-1', position: 1, kind: 'instruction', title: 'Look first', required: true }],
  automation: { onAlert: false },
};

function backend() {
  server.use(
    http.get('*/api/runbooks', () =>
      HttpResponse.json({ success: true, data: { items: [runbook], total: 1, page: 1, perPage: 200 } }),
    ),
    http.get('*/api/runbooks/catalog', () =>
      HttpResponse.json({ success: true, data: { actions: [], diagnostics: [], targets: [] } }),
    ),
    http.get('*/api/runbooks/principals', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/runbooks/rb-1', () => HttpResponse.json({ success: true, data: runbook })),
    http.get('*/api/environments', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/alerts/conditions', () => HttpResponse.json({ success: true, data: { sources: [] } })),
  );
}

function renderAt(path: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/runbooks" element={<RunbooksPage />} />
        <Route path="/runbooks/:id" element={<RunbookEditRedirect />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RunbooksPage', () => {
  beforeEach(() => {
    for (const id of ['header-slot', 'pagehead-slot']) {
      const slot = document.createElement('div');
      slot.id = id;
      document.body.appendChild(slot);
    }
  });

  afterEach(() => {
    document.getElementById('header-slot')?.remove();
    document.getElementById('pagehead-slot')?.remove();
  });

  it('puts the Create button in the page head, and it opens the editor modal', async () => {
    backend();
    grantPermissions('*');
    const user = userEvent.setup();
    renderAt('/runbooks');

    const band = document.getElementById('pagehead-slot') as HTMLElement;
    const create = await within(band).findByRole('button', { name: 'Create runbook' });
    expect(within(band).getByText(/Step-by-step procedures/)).toBeInTheDocument();
    expect(await screen.findByText('Restart loop')).toBeInTheDocument();

    await user.click(create);
    expect(await screen.findByRole('dialog', { name: 'Create runbook' })).toBeInTheDocument();
  });

  it('opens the editor for ?edit=<id> with the runbook loaded, and from a row', async () => {
    backend();
    grantPermissions('*');
    const user = userEvent.setup();
    renderAt('/runbooks?edit=rb-1');

    await screen.findByRole('dialog', { name: 'Edit runbook' });
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Restart loop'));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

    await user.click(await screen.findByRole('button', { name: 'Edit' }));
    expect(await screen.findByRole('dialog', { name: 'Edit runbook' })).toBeInTheDocument();
  });

  it('keeps the old /runbooks/<id> link working by opening the modal', async () => {
    backend();
    grantPermissions('*');
    renderAt('/runbooks/rb-1');

    await screen.findByRole('dialog', { name: 'Edit runbook' });
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Restart loop'));
  });

  it('renders Create disabled without runbooks.manage and opens nothing', async () => {
    backend();
    grantPermissions(['runbooks.view']);
    const user = userEvent.setup();
    renderAt('/runbooks');

    const band = document.getElementById('pagehead-slot') as HTMLElement;
    const create = await within(band).findByRole('button', { name: 'Create runbook' });
    expect(create).toBeDisabled();
    await user.click(create);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
