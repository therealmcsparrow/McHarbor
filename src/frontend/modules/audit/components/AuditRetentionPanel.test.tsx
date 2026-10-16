// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { delay, http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuditRetentionPanel } from './AuditRetentionPanel';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const view = {
  defaultDays: 90,
  severities: ['info', 'success', 'warning', 'error'],
  overrides: { info: 30, success: null, warning: null, error: 365 },
  effective: { info: 30, success: 90, warning: 90, error: 365 },
  maxDays: 3650,
};

function renderPanel() {
  return renderWithProviders(
    <MemoryRouter>
      <AuditRetentionPanel />
    </MemoryRouter>,
  );
}

describe('AuditRetentionPanel', () => {
  const saves = vi.fn();

  beforeEach(() => {
    saves.mockReset();
    grantPermissions(['audit.view', 'settings.manage']);
    server.use(
      http.get('*/api/audit/retention', () => HttpResponse.json({ success: true, data: view })),
      http.put('*/api/audit/retention', async ({ request }) => {
        saves(await request.json());
        return HttpResponse.json({ success: true, data: view });
      }),
    );
  });

  it('sends only the severity the operator changed, as null when it returns to the default', async () => {
    const user = userEvent.setup();
    renderPanel();
    const save = await screen.findByRole('button', { name: 'Save retention' });
    expect(save).toBeDisabled();
    await user.click(screen.getByLabelText('Use default', { selector: '#audit-retention-inherit-info' }));
    await user.click(save);
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves).toHaveBeenCalledWith({ overrides: { info: null } });
  });

  it('renders the save disabled, naming settings.manage, when the permission is missing', async () => {
    grantPermissions(['audit.view']);
    const user = userEvent.setup();
    renderPanel();
    const save = await screen.findByRole('button', { name: 'Save retention' });
    expect(save).toBeDisabled();
    expect(screen.getByLabelText('Use default', { selector: '#audit-retention-inherit-info' })).toBeDisabled();
    await user.hover(save.parentElement ?? save);
    expect(await screen.findAllByText('Requires the settings.manage permission')).not.toHaveLength(0);
    expect(saves).not.toHaveBeenCalled();
  });

  it('shows the server message in full when the save is refused', async () => {
    server.use(
      http.put('*/api/audit/retention', () =>
        HttpResponse.json(
          { success: false, error: 'Retention for error out of range: keep it under ten years' },
          { status: 400 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(await screen.findByLabelText('Use default', { selector: '#audit-retention-inherit-warning' }));
    await user.click(screen.getByRole('button', { name: 'Save retention' }));
    expect(await screen.findByText('Audit retention could not be saved')).toBeInTheDocument();
    expect(screen.getByText('Retention for error out of range: keep it under ten years')).toBeInTheDocument();
  });

  it('sends one request on a double click while the save is in flight', async () => {
    server.use(
      http.put('*/api/audit/retention', async ({ request }) => {
        saves(await request.json());
        await delay('infinite');
        return HttpResponse.json({ success: true, data: view });
      }),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(await screen.findByLabelText('Use default', { selector: '#audit-retention-inherit-success' }));
    await user.dblClick(screen.getByRole('button', { name: 'Save retention' }));
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves).toHaveBeenCalledTimes(1);
  });
});
