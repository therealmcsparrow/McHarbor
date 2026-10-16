// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { CapabilityInstall } from './CapabilityInstall';
import { COMPUTE_PERM } from '../permissions';

const gate = vi.hoisted(() => ({ denied: new Set<string>() }));
const post = vi.hoisted(() => vi.fn());

vi.mock('@resources/hooks/usePermissionGate', () => ({
  usePermissionGate: (key: string | string[]) => {
    const keys = Array.isArray(key) ? key : [key];
    const denied = keys.some((k) => gate.denied.has(k));
    return { allowed: !denied, denied, reason: denied ? `Requires the ${keys.join(', ')} permission` : '' };
  },
}));
vi.mock('@core/api/client', () => ({ api: { post }, envelopeStatus: () => undefined }));

const item = { key: 'qemu_img', present: false, installable: true, remediation: 'sudo apt install qemu-utils' };

function mount() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <CapabilityInstall environmentId="env1" item={item} label="qemu-img" />
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

describe('CapabilityInstall', () => {
  beforeEach(() => {
    gate.denied.clear();
    post.mockReset();
  });

  it('asks before installing, and sends only the item', async () => {
    post.mockResolvedValue({ success: true, data: { item: 'qemu_img', packages: ['qemu-utils'] } });
    const user = userEvent.setup();
    mount();

    await user.click(screen.getByRole('button', { name: 'Install' }));
    expect(screen.getByText('sudo apt install qemu-utils')).toBeInTheDocument();
    expect(post).not.toHaveBeenCalled();

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Install' }));
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(post).toHaveBeenCalledWith('/compute/environments/env1/capability/install', { item: 'qemu_img' });
  });

  it('is disabled and names compute.manage when the permission is missing', async () => {
    gate.denied.add(COMPUTE_PERM.MANAGE);
    const user = userEvent.setup();
    mount();
    const button = screen.getByRole('button', { name: /Install/ });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement ?? button);
    expect((await screen.findAllByText(/compute.manage/)).length).toBeGreaterThan(0);
    expect(post).not.toHaveBeenCalled();
  });

  it('keeps the dialog open with the host message and the package manager output on failure', async () => {
    post.mockResolvedValue({
      success: false,
      code: 'err.compute.agent_failed',
      error: 'installing qemu-utils failed: exit status 100: E: Unable to locate package qemu-utils',
      data: { item: 'qemu_img', output: '$ apt-get install -y qemu-utils\nE: Unable to locate package qemu-utils' },
    });
    const user = userEvent.setup();
    mount();

    await user.click(screen.getByRole('button', { name: 'Install' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Install' }));

    expect(await screen.findByText(/exit status 100: E: Unable to locate package qemu-utils/)).toBeInTheDocument();
    expect(screen.getByText(/\$ apt-get install -y qemu-utils/)).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends one request on a double click while the install is running', async () => {
    let finish: (value: unknown) => void = () => undefined;
    post.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    const user = userEvent.setup();
    mount();

    await user.click(screen.getByRole('button', { name: 'Install' }));
    const confirm = within(screen.getByRole('dialog')).getByRole('button', { name: 'Install' });
    await user.dblClick(confirm);
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    finish({ success: true, data: { item: 'qemu_img', packages: ['qemu-utils'] } });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(post).toHaveBeenCalledTimes(1);
  });
});
