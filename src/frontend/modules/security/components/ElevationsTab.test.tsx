// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { ElevationsTab } from './ElevationsTab';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const catalog = [
  {
    key: 'vm.manage',
    category: 'vm',
    action: 'manage',
    description: 'Manage virtual machines',
    origin: 'addon-kvm',
    elevatable: true,
    source: 'default',
  },
  {
    key: 'users.manage',
    category: 'users',
    action: 'manage',
    description: 'Manage users',
    origin: 'host',
    elevatable: false,
    source: 'default',
  },
];

describe('Elevations tab', () => {
  let created: unknown[];
  let policy: unknown[];

  beforeEach(() => {
    created = [];
    policy = [];
    server.use(
      http.get('*/api/elevations/elevatable', () => HttpResponse.json({ success: true, data: catalog })),
      http.get('*/api/elevations/mine', () =>
        HttpResponse.json({
          success: true,
          data: {
            requests: [
              {
                id: 'r1',
                requesterId: 'u1',
                permissionKey: 'vm.manage',
                permissionLabel: 'Manage virtual machines',
                justification: 'migrate',
                requestedMinutes: 30,
                status: 'pending',
                createdAt: '2026-09-27T00:00:00Z',
              },
            ],
          },
        }),
      ),
      http.get('*/api/elevations/pending', () => HttpResponse.json({ success: true, data: { requests: [] } })),
      http.post('*/api/elevations', async ({ request }) => {
        created.push(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'r2' } }, { status: 201 });
      }),
      http.put('*/api/elevations/elevatable', async ({ request }) => {
        policy.push(await request.json());
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
  });

  it('offers requestable permissions by their readable name, without the catalog permission', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<ElevationsTab />);

    expect(await screen.findByText('Manage virtual machines', { selector: 'p' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Permission' }));
    expect(await screen.findByRole('button', { name: /Manage virtual machines/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Manage users/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Manage virtual machines/ }));
    await user.type(screen.getByPlaceholderText('Why do you need this?'), 'incident');
    await user.click(screen.getByRole('button', { name: 'Request access' }));
    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toMatchObject({
      permissionKey: 'vm.manage',
      justification: 'incident',
    });
    expect(screen.queryByText('Requestable permissions')).not.toBeInTheDocument();
  });

  it('lets an approver switch a permission on and names each by its description', async () => {
    grantPermissions(['users.manage']);
    const user = userEvent.setup();
    renderWithProviders(<ElevationsTab />);

    const toggle = await screen.findByRole('switch', {
      name: 'Allow requesting Manage users',
    });
    expect(toggle).not.toBeChecked();
    await user.click(toggle);
    await waitFor(() => expect(policy).toEqual([{ key: 'users.manage', elevatable: true }]));

    const list = screen.getByRole('switch', { name: 'Allow requesting Manage virtual machines' }).closest('li');
    expect(within(list as HTMLElement).getByText('vm.manage · addon-kvm')).toBeInTheDocument();
  });
});
