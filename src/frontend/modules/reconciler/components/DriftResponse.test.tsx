// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { ModeDialog } from './ModeDialog';
import { ProposalDialog } from './ProposalDialog';
import type { DesiredState } from '../hooks/useReconciler';
import type { Proposal } from '../hooks/useProposals';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const web: DesiredState = {
  id: 'ds1',
  name: 'web',
  description: '',
  kind: 'container',
  stackName: '',
  containerName: 'web',
  imageRef: 'nginx:1',
  desiredStatus: 'running',
  restartPolicy: '',
  lastReconcile: null,
  driftDetected: true,
  createdAt: '2026-09-26T00:00:00Z',
  mode: 'observe',
};

const plan: Proposal = {
  id: 'p1',
  desiredStateId: 'ds1',
  desiredName: 'web',
  kind: 'container',
  envId: 'e1',
  origin: 'propose',
  status: 'pending',
  fingerprint: 'f',
  createdAt: '2026-09-26T10:00:00Z',
  results: [],
  report: {
    desiredStateId: 'ds1',
    hasDrift: true,
    checkedAt: '2026-09-26T10:00:00Z',
    diffs: [{ class: 'state', field: 'status', expected: 'running', actual: 'exited' }],
  },
  steps: [
    { action: 'start', target: 'web', fields: ['status'] },
    { action: 'manual', target: 'web', fields: ['port.80/tcp'], reason: 'ports' },
  ],
};

function held() {
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { gate, release: () => release() };
}

describe('Enforce mode', () => {
  const enforces = vi.fn();
  beforeEach(() => {
    enforces.mockReset();
    grantPermissions(['reconciler.view', 'reconciler.manage', 'reconciler.enforce']);
    server.use(
      http.post('*/api/reconciler/:id/enforce', ({ params }) => {
        enforces(params.id);
        return HttpResponse.json({ success: true, data: { ...web, mode: 'enforce' } });
      }),
    );
  });

  async function chooseEnforce(user: ReturnType<typeof userEvent.setup>) {
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'When it drifts' }));
    await user.click(await screen.findByRole('button', { name: 'Enforce' }));
    return within(dialog).getByRole('button', { name: 'Let McHarbor fix it automatically' });
  }

  it('asks nothing of the server until enforce is chosen and confirmed, warns first, then closes', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<ModeDialog state={web} onOpenChange={onOpenChange} />);
    const confirm = await chooseEnforce(user);
    expect(screen.getByText(/changes containers without asking anyone/)).toBeInTheDocument();
    expect(enforces).not.toHaveBeenCalled();
    await user.click(confirm);
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(enforces).toHaveBeenCalledTimes(1);
  });

  it('is disabled and names reconciler.enforce for someone who can only manage', async () => {
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    const user = userEvent.setup();
    renderWithProviders(<ModeDialog state={web} onOpenChange={vi.fn()} />);
    const confirm = await chooseEnforce(user);
    expect(confirm).toBeDisabled();
    await user.click(confirm);
    expect(enforces).not.toHaveBeenCalled();
    await user.hover(confirm.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the reconciler.enforce permission')).not.toHaveLength(0);
  });

  it('keeps the dialog and the server message on failure, and sends one request on a double click', async () => {
    const { gate, release } = held();
    server.use(
      http.post('*/api/reconciler/:id/enforce', async ({ params }) => {
        enforces(params.id);
        await gate;
        return HttpResponse.json({ success: false, error: 'enforcing web: database is locked' }, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<ModeDialog state={web} onOpenChange={onOpenChange} />);
    const confirm = await chooseEnforce(user);
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    expect(await screen.findByText('enforcing web: database is locked')).toBeInTheDocument();
    expect(enforces).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});

describe('Approving a plan', () => {
  const approvals = vi.fn();
  beforeEach(() => {
    approvals.mockReset();
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    server.use(
      http.post('*/api/reconciler/proposals/:id/approve', ({ params }) => {
        approvals(params.id);
        return HttpResponse.json({
          success: true,
          data: {
            stale: false,
            proposal: {
              ...plan,
              status: 'applied',
              resolvedBy: 'u1',
              results: [
                { action: 'start', target: 'web', status: 'done' },
                { action: 'manual', target: 'web', status: 'manual' },
              ],
            },
          },
        });
      }),
    );
  });

  it('shows the whole plan, manual steps with their reason, and applies only after a second confirmation', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ProposalDialog proposal={plan} onOpenChange={vi.fn()} />);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/Published ports cannot be changed by recreating/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Approve and apply' }));
    expect(approvals).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Apply now' }));
    await waitFor(() => expect(approvals).toHaveBeenCalledTimes(1));
    expect(await within(dialog).findByText('Applied')).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Apply now' })).toBeNull();
  });

  it('is disabled and names reconciler.manage without it', async () => {
    grantPermissions(['reconciler.view']);
    const user = userEvent.setup();
    renderWithProviders(<ProposalDialog proposal={plan} onOpenChange={vi.fn()} />);
    const approve = within(await screen.findByRole('dialog')).getByRole('button', { name: 'Approve and apply' });
    expect(approve).toBeDisabled();
    await user.hover(approve.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the reconciler.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog and the server message on failure, and applies once on a double click', async () => {
    const { gate, release } = held();
    server.use(
      http.post('*/api/reconciler/proposals/:id/approve', async ({ params }) => {
        approvals(params.id);
        await gate;
        return HttpResponse.json({ success: false, error: 'applying plan p1: agent disconnected' }, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<ProposalDialog proposal={plan} onOpenChange={onOpenChange} />);
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Approve and apply' }));
    const apply = within(dialog).getByRole('button', { name: 'Apply now' });
    await user.dblClick(apply);
    await user.click(apply);
    release();
    expect(await within(dialog).findByText('applying plan p1: agent disconnected')).toBeInTheDocument();
    expect(approvals).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it('shows a stale plan as replaced, with the fresh plan for review, instead of claiming success', async () => {
    server.use(
      http.post('*/api/reconciler/proposals/:id/approve', () =>
        HttpResponse.json({
          success: true,
          data: {
            stale: true,
            proposal: { ...plan, id: 'p2', steps: [{ action: 'recreate', target: 'web', fields: ['image'] }] },
          },
        }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<ProposalDialog proposal={plan} onOpenChange={vi.fn()} />);
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Approve and apply' }));
    await user.click(within(dialog).getByRole('button', { name: 'Apply now' }));
    expect(await within(dialog).findByText(/The drift changed since this plan was written/)).toBeInTheDocument();
    expect(within(dialog).getByText('Recreate')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Approve and apply' })).toBeEnabled();
  });
});
