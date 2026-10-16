// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { describeDestructiveDialog } from '../../../test/destructive';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostUpdateApplyDialog } from '@resources/components/host/HostUpdateApplyDialog';
import type { HostTarget } from '@resources/hooks/useHostUpdates';

const ENV_TARGET: HostTarget = { kind: 'environment', envId: 'env-1' };

const PATH = '*/api/system/os-updates/apply';

describeDestructiveDialog({
  name: 'HostUpdateApplyDialog',
  method: 'post',
  path: PATH,
  permission: 'host_updates.manage',
  confirmLabel: 'Update host',
  body: { confirm: true, withNewPackages: false, skipUnavailable: false, removeUnused: false },
  reportsSuccess: false,
  render: ({ onOpenChange }) => (
    <HostUpdateApplyDialog target={ENV_TARGET} open count={3} onOpenChange={onOpenChange} onStarted={vi.fn()} />
  ),
});

describe('HostUpdateApplyDialog options', () => {
  const calls = vi.fn();

  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['host_updates.manage']);
    server.use(
      http.post(PATH, async ({ request }) => {
        calls(await request.json());
        return HttpResponse.json({ success: true, data: { jobId: 'job-1' } });
      }),
    );
  });

  it('sends skipUnavailable when the operator ticks Skip unavailable packages', async () => {
    const user = userEvent.setup();
    const onStarted = vi.fn();
    renderWithProviders(
      <HostUpdateApplyDialog target={ENV_TARGET} open count={1} onOpenChange={vi.fn()} onStarted={onStarted} />,
    );
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('checkbox', { name: 'Skip unavailable packages' }));
    await user.click(within(dialog).getByRole('button', { name: 'Update host' }));
    await waitFor(() => expect(onStarted).toHaveBeenCalledWith('job-1'));
    expect(calls).toHaveBeenCalledWith({
      confirm: true,
      withNewPackages: false,
      skipUnavailable: true,
      removeUnused: false,
    });
  });

  it('sends removeUnused only when the operator ticks Remove unused packages', async () => {
    const user = userEvent.setup();
    const onStarted = vi.fn();
    renderWithProviders(
      <HostUpdateApplyDialog target={ENV_TARGET} open count={1} onOpenChange={vi.fn()} onStarted={onStarted} />,
    );
    const dialog = await screen.findByRole('dialog');
    const remove = within(dialog).getByRole('checkbox', { name: 'Remove unused packages' });
    expect(remove).not.toBeChecked();
    await user.click(remove);
    await user.click(within(dialog).getByRole('button', { name: 'Update host' }));
    await waitFor(() => expect(onStarted).toHaveBeenCalledWith('job-1'));
    expect(calls).toHaveBeenCalledWith({
      confirm: true,
      withNewPackages: false,
      skipUnavailable: false,
      removeUnused: true,
    });
  });

  it('starts from the saved defaults and lets this run turn them off', async () => {
    const user = userEvent.setup();
    const onStarted = vi.fn();
    renderWithProviders(
      <HostUpdateApplyDialog
        target={ENV_TARGET}
        open
        count={1}
        defaults={{ withNewPackages: true, skipUnavailable: true, removeUnused: true }}
        onOpenChange={vi.fn()}
        onStarted={onStarted}
      />,
    );
    const dialog = await screen.findByRole('dialog');
    const skip = within(dialog).getByRole('checkbox', { name: 'Skip unavailable packages' });
    expect(skip).toBeChecked();
    expect(within(dialog).getByRole('checkbox', { name: 'Remove unused packages' })).toBeChecked();
    await user.click(skip);
    await user.click(within(dialog).getByRole('button', { name: 'Update host' }));
    await waitFor(() => expect(onStarted).toHaveBeenCalledWith('job-1'));
    expect(calls).toHaveBeenCalledWith({
      confirm: true,
      withNewPackages: true,
      skipUnavailable: false,
      removeUnused: true,
    });
  });

  it('says when the agent is too old to honour the option', async () => {
    renderWithProviders(
      <HostUpdateApplyDialog
        target={ENV_TARGET}
        open
        count={1}
        recoveryUnsupported
        onOpenChange={vi.fn()}
        onStarted={vi.fn()}
      />,
    );
    expect(await screen.findByText(/predates Skip unavailable packages/)).toBeInTheDocument();
  });
});
