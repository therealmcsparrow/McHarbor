// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BreakGlassSelfPanel } from './BreakGlassSelfPanel';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

describe('BreakGlassSelfPanel', () => {
  const activations = vi.fn();
  const stepUps = vi.fn();

  beforeEach(() => {
    activations.mockReset();
    stepUps.mockReset();
    server.use(
      http.get('*/api/break-glass/status', () =>
        HttpResponse.json({ success: true, data: { designated: true, maxMinutes: 60, active: null } }),
      ),
      http.post('*/api/auth/sudo', async ({ request }) => {
        stepUps(await request.json());
        return HttpResponse.json({ success: true, data: { expiresAt: '', expiresIn: 300 } });
      }),
      http.post('*/api/break-glass/activate', async ({ request }) => {
        activations(await request.json());
        return HttpResponse.json({
          success: true,
          data: {
            id: 's1',
            userId: 'u1',
            username: 'u',
            reason: 'x',
            startedAt: '',
            expiresAt: '',
            endedAt: null,
            endedBy: null,
          },
        });
      }),
    );
  });

  it('refuses to open without a real reason and a window within the designation', async () => {
    const user = userEvent.setup();
    renderWithProviders(<BreakGlassSelfPanel />);
    await user.click(await screen.findByRole('button', { name: 'Open emergency access' }));
    const dialog = await screen.findByRole('dialog');
    const confirm = within(dialog).getByRole('button', { name: 'Open emergency access' });
    expect(confirm).toBeDisabled();

    await user.type(within(dialog).getByRole('textbox'), 'db down');
    expect(confirm).toBeDisabled();
    await user.type(within(dialog).getByRole('textbox'), ', primary lost');
    const minutes = within(dialog).getByRole('spinbutton');
    await user.clear(minutes);
    await user.type(minutes, '120');
    expect(confirm).toBeDisabled();
    await user.clear(minutes);
    await user.type(minutes, '15');
    expect(confirm).toBeDisabled();
    await user.type(within(dialog).getByLabelText(/Your password/), 'hunter2');
    expect(confirm).toBeEnabled();
    expect(activations).not.toHaveBeenCalled();

    await user.click(confirm);
    await waitFor(() => expect(activations).toHaveBeenCalledTimes(1));
    expect(stepUps).toHaveBeenCalledWith({ scope: 'break_glass', password: 'hunter2' });
    expect(activations).toHaveBeenCalledWith({ reason: 'db down, primary lost', minutes: 15 });
  });
});
