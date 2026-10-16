// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { NetworkCreateDialog } from './NetworkCreateDialog';

describe('network create IPv6', () => {
  beforeEach(() => grantPermissions(['networks.manage']));
  afterEach(() => resetPermissions());

  it('sends enableIPv6 and the IPv6 subnet only when the switch is on', async () => {
    const bodies: unknown[] = [];
    server.use(
      http.post('*/api/networks', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: { Id: 'n1' } }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<NetworkCreateDialog open onOpenChange={() => {}} />);
    await user.type(screen.getByPlaceholderText(/network/i), 'lan6');
    await user.type(
      screen.getAllByRole('textbox').find((el) => el.getAttribute('placeholder')?.match(/subnet/i))!,
      '2001:db8:1::/64',
    );
    await user.click(screen.getByRole('switch', { name: 'Enable IPv6' }));
    await user.click(screen.getByRole('button', { name: /create/i }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({
      name: 'lan6',
      enableIPv6: true,
      ipam: { Config: [{ Subnet: '2001:db8:1::/64' }] },
    });
  });

  it('omits enableIPv6 when the switch stays off', async () => {
    const bodies: Record<string, unknown>[] = [];
    server.use(
      http.post('*/api/networks', async ({ request }) => {
        bodies.push((await request.json()) as Record<string, unknown>);
        return HttpResponse.json({ success: true, data: { Id: 'n2' } }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<NetworkCreateDialog open onOpenChange={() => {}} />);
    await user.type(screen.getByPlaceholderText(/network/i), 'lan4');
    await user.click(screen.getByRole('button', { name: /create/i }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]?.enableIPv6).toBeUndefined();
  });
});
