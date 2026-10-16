// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { CreateServicePrincipalDialog } from './CreateServicePrincipalDialog';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByPlaceholderText('Username'), 'runbook-bot');
  await user.click(screen.getByRole('button', { name: 'Role' }));
  await user.click(await screen.findByRole('button', { name: 'Operator' }));
}

describe('CreateServicePrincipalDialog', () => {
  beforeEach(() => {
    server.use(
      http.get('*/api/roles', () =>
        HttpResponse.json({ success: true, data: [{ id: 'role_operator', name: 'Operator', permissions: [] }] }),
      ),
    );
  });

  it('asks for no password and creates a service principal with the chosen role', async () => {
    const bodies: unknown[] = [];
    server.use(
      http.post('*/api/users', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'svc-1', username: 'runbook-bot', kind: 'service' } });
      }),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<CreateServicePrincipalDialog open onOpenChange={onOpenChange} />);

    expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
    await fill(user);
    await user.click(screen.getByRole('button', { name: 'Create' }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({ username: 'runbook-bot', roleId: 'role_operator', isActive: true, kind: 'service' });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it('renders Create disabled without users.manage, names the key, and sends nothing', async () => {
    const calls = vi.fn();
    server.use(
      http.post('*/api/users', () => {
        calls();
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
    grantPermissions(['users.view']);
    const user = userEvent.setup();
    renderWithProviders(<CreateServicePrincipalDialog open onOpenChange={() => undefined} />);

    await fill(user);
    const create = screen.getByRole('button', { name: 'Create' });
    expect(create).toBeDisabled();
    await user.click(create);
    expect(calls).not.toHaveBeenCalled();
    await user.hover(create.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the users.manage permission')).not.toHaveLength(0);
  });

  it("keeps the server's refusal intact and the dialog open", async () => {
    const message = 'A service principal needs a role';
    server.use(
      http.post('*/api/users', () =>
        HttpResponse.json({ success: false, error: message, code: 'err.user.service_principal_role' }, { status: 400 }),
      ),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<CreateServicePrincipalDialog open onOpenChange={onOpenChange} />);

    await fill(user);
    await user.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
