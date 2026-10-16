// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { DeviceApprovalForm } from './DeviceApprovalForm';
import { normalizeUserCode } from '../hooks/useDeviceSignIn';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const request = {
  userCode: 'BCDF-GHJK',
  host: 'laptop',
  osUser: 'carlo',
  expiresAt: '2026-09-26T12:10:00Z',
  createdAt: '2026-09-26T12:00:00Z',
};

const options = {
  roles: [{ id: 'r-read', name: 'Read only', description: '', permissions: ['containers.view'], grantable: true }],
  environments: [],
  permissions: ['containers.view'],
  wildcard: false,
};

async function pickRole(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Roles' }));
  const roles = await screen.findByRole('listbox', { name: 'Roles' });
  await user.click(within(roles).getByRole('option', { name: /Read only/ }));
}

describe('DeviceApprovalForm', () => {
  const approveCalls = vi.fn();
  const denyCalls = vi.fn();

  beforeEach(() => {
    approveCalls.mockReset();
    denyCalls.mockReset();
    server.use(
      http.get('*/api/api-keys/options', () => HttpResponse.json({ success: true, data: options })),
      http.post('*/api/device/requests/BCDF-GHJK/deny', () => {
        denyCalls();
        return HttpResponse.json({ success: true, data: { status: 'denied' } });
      }),
    );
  });

  it('approves once, for this code, with exactly the grants chosen', async () => {
    grantPermissions(['api_keys.manage']);
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/device/requests/BCDF-GHJK/approve', async ({ request: req }) => {
        approveCalls(await req.json());
        await held;
        return HttpResponse.json({ success: true, data: { status: 'approved' } });
      }),
    );
    const onDecided = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<DeviceApprovalForm request={request} onDecided={onDecided} />);
    expect(screen.getByText('laptop')).toBeInTheDocument();
    await pickRole(user);
    const approve = screen.getByRole('button', { name: 'Approve' });
    await user.dblClick(approve);
    release();
    await waitFor(() => expect(onDecided).toHaveBeenCalledWith(true));
    expect(approveCalls).toHaveBeenCalledTimes(1);
    expect(approveCalls).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'mcharbor on laptop', roleIds: ['r-read'], scopes: [], environmentIds: [] }),
    );
  });

  it('renders both decisions disabled, naming api_keys.manage, without it', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<DeviceApprovalForm request={request} onDecided={() => undefined} />);
    const deny = screen.getByRole('button', { name: 'Deny' });
    expect(deny).toBeDisabled();
    await user.click(deny);
    expect(denyCalls).not.toHaveBeenCalled();
    await user.hover(deny.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the api_keys.manage permission')).not.toHaveLength(0);
  });

  it("shows the server's refusal in full and stays open", async () => {
    grantPermissions(['api_keys.manage']);
    const message = 'API key grant exceeds your own permissions: containers.manage is not yours to give';
    server.use(
      http.post('*/api/device/requests/BCDF-GHJK/approve', () =>
        HttpResponse.json(
          { success: false, error: message, code: 'err.api_key.grant_exceeds_caller' },
          { status: 400 },
        ),
      ),
    );
    const onDecided = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<DeviceApprovalForm request={request} onDecided={onDecided} />);
    await pickRole(user);
    await user.click(screen.getByRole('button', { name: 'Approve' }));
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(onDecided).not.toHaveBeenCalled();
  });

  it('denies with one request', async () => {
    grantPermissions(['api_keys.manage']);
    const onDecided = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<DeviceApprovalForm request={request} onDecided={onDecided} />);
    await user.click(screen.getByRole('button', { name: 'Deny' }));
    await waitFor(() => expect(onDecided).toHaveBeenCalledWith(false));
    expect(denyCalls).toHaveBeenCalledTimes(1);
    expect(approveCalls).not.toHaveBeenCalled();
  });

  it('accepts a code the way people type it', () => {
    expect(normalizeUserCode('bcdf ghjk')).toBe('BCDF-GHJK');
    expect(normalizeUserCode('BCDF-GHJK')).toBe('BCDF-GHJK');
    expect(normalizeUserCode('b-c-d-f-g-h-j-k')).toBe('BCDF-GHJK');
  });
});
