// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PushDeliveryCard } from './PushDeliveryCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

describe('PushDeliveryCard', () => {
  const regenerations = vi.fn();

  beforeEach(() => {
    regenerations.mockReset();
    server.use(
      http.post('*/api/push/vapid/regenerate', () => {
        regenerations();
        return HttpResponse.json({ success: true, data: { publicKey: 'k', removedSubscriptions: 2 } });
      }),
    );
  });

  it('disables regenerate and names settings.manage when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<PushDeliveryCard />);
    const button = screen.getByRole('button', { name: 'Regenerate VAPID key' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the settings.manage permission')).not.toHaveLength(0);
    expect(regenerations).not.toHaveBeenCalled();
  });

  it('warns that every device subscription stops receiving pushes and sends nothing before confirming', async () => {
    grantPermissions(['settings.manage']);
    const user = userEvent.setup();
    renderWithProviders(<PushDeliveryCard />);
    await user.click(screen.getByRole('button', { name: 'Regenerate VAPID key' }));
    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByText(/Every existing device subscription stops receiving pushes until it re-subscribes/),
    ).toBeInTheDocument();
    expect(regenerations).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Regenerate key' }));
    await waitFor(() => expect(regenerations).toHaveBeenCalledTimes(1));
  });
});
