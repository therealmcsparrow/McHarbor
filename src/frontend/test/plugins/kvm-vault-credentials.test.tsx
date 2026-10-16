// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  PasswordField,
  VaultKeysField,
  type VaultCredentialForm,
} from '../../../plugins/Plugins/addon-kvm/frontend/src/components/create-vm/VaultCredentialFields';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const listed = vi.fn();

function Harness() {
  const [password, setPassword] = useState('');
  const [passwordFromVault, setPasswordFromVault] = useState(false);
  const [passwordSecret, setPasswordSecret] = useState('');
  const [sshKeySecrets, setSshKeySecrets] = useState<string[]>([]);
  const form: VaultCredentialForm = {
    environmentId: 'envA',
    password,
    setPassword,
    passwordFromVault,
    setPasswordFromVault,
    passwordSecret,
    setPasswordSecret,
    sshKeySecrets,
    setSshKeySecrets,
  };
  return (
    <>
      <PasswordField form={form} />
      <VaultKeysField form={form} />
      <output data-testid="state">{JSON.stringify({ passwordFromVault, passwordSecret, sshKeySecrets })}</output>
    </>
  );
}

const state = () => JSON.parse(screen.getByTestId('state').textContent ?? '{}');

describe('addon-kvm vault credentials in the create wizard', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  beforeEach(() => {
    listed.mockReset();
    server.use(
      http.get('*/api/secrets', ({ request }) => {
        listed(new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          // The real list route is paginated, not a bare array.
          data: {
            items: [
              { id: 's1', name: 'VM_PASSWORD', kind: 'password', scope: 'environment' },
              { id: 's2', name: 'ADMIN_KEYS', kind: 'ssh_key', scope: 'global' },
            ],
            total: 2,
            page: 1,
            perPage: 1000,
            totalPages: 1,
          },
        });
      }),
    );
  });

  it('swaps the typed password for a vault secret and says the copy is taken once', async () => {
    grantPermissions(['vm.manage', 'secrets.view']);
    const user = userEvent.setup();
    renderWithProviders(<Harness />);

    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    await user.click(screen.getByRole('switch', { name: 'From the vault' }));

    expect(screen.queryByLabelText('Password', { selector: 'input' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Password secret' }));
    await user.click(await screen.findByRole('button', { name: /VM_PASSWORD/ }));
    expect(state()).toMatchObject({ passwordFromVault: true, passwordSecret: 'VM_PASSWORD' });
    expect(listed).toHaveBeenCalledWith('envA');
    expect(screen.getAllByText(/Copied from the vault once/).length).toBeGreaterThan(0);
  });

  it('keeps the vault disabled and names secrets.view when the user lacks it', async () => {
    grantPermissions(['vm.manage']);
    renderWithProviders(<Harness />);

    expect(screen.getByRole('switch', { name: 'From the vault' })).toBeDisabled();
    expect(screen.getAllByText(/secrets\.view/).length).toBeGreaterThan(0);
    await waitFor(() => expect(listed).not.toHaveBeenCalled());
  });
});
