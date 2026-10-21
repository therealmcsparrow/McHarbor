// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { NarrationSettingsDialog } from './NarrationSettingsDialog';

describe('NarrationSettingsDialog load failure', () => {
  beforeEach(() => {
    grantPermissions(['incidents.view', 'incidents.manage']);
  });

  it('shows the server message in an error state instead of spinning forever', async () => {
    server.use(
      http.get('*/api/incidents/settings', () =>
        HttpResponse.json(
          { success: false, error: 'Narration settings store is unavailable', code: 'err.internal' },
          { status: 500 },
        ),
      ),
    );
    renderWithProviders(<NarrationSettingsDialog envId="e1" onOpenChange={() => {}} />);
    const dialog = await screen.findByRole('dialog');
    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('Narration settings store is unavailable');
    expect(within(dialog).queryByRole('button', { name: 'Save' })).toBeNull();
  });

  it('retries the settings request from the error state and then shows the form', async () => {
    let calls = 0;
    server.use(
      http.get('*/api/incidents/settings', () => {
        calls += 1;
        if (calls === 1) {
          return HttpResponse.json(
            { success: false, error: 'Narration settings store is unavailable', code: 'err.internal' },
            { status: 500 },
          );
        }
        return HttpResponse.json({
          success: true,
          data: { envId: 'e1', narrateOptIn: false, proactive: false, hourlyCap: 5, modelConfigured: false },
        });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<NarrationSettingsDialog envId="e1" onOpenChange={() => {}} />);
    const dialog = await screen.findByRole('dialog');
    await user.click(await within(dialog).findByRole('button', { name: 'Try again' }));
    expect(await within(dialog).findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(calls).toBe(2);
    expect(within(dialog).queryByText('Narration settings store is unavailable')).toBeNull();
  });
});
