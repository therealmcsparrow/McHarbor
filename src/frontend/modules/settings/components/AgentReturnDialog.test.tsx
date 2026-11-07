// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AgentRow } from '@resources/hooks/useAgentInventory';
import { AgentStatusCell } from './AgentInventoryColumns';
import { AgentReturnDialog } from './AgentReturnDialog';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const agent: AgentRow = {
  envId: 'env-9',
  name: 'homelab01',
  version: '2026.11.02-rc.1',
  outdated: false,
  updatable: true,
};
const BOTH = ['updates.manage', 'environments.manage'];

describe('AgentReturnDialog', () => {
  const calls = vi.fn();
  const onOpenChange = vi.fn();

  function answer(response: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/updates/agents/env-9/return', () => {
        calls();
        return response();
      }),
    );
  }

  function mount() {
    return renderWithProviders(<AgentReturnDialog agent={agent} open onOpenChange={onOpenChange} />);
  }

  async function confirm() {
    const dialog = await screen.findByRole('dialog');
    return within(dialog).getByRole('button', { name: 'Return and pin' });
  }

  beforeEach(() => {
    calls.mockReset();
    onOpenChange.mockReset();
    grantPermissions(BOTH);
    answer(() => HttpResponse.json({ success: true, data: {} }));
  });

  it('sends nothing when cancelled', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(calls).not.toHaveBeenCalled();
  });

  it('sends exactly one request, only after confirmation, then closes', async () => {
    const user = userEvent.setup();
    mount();
    expect(calls).not.toHaveBeenCalled();
    await user.click(await confirm());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledTimes(1);
  });

  it('reports a started move as started, not finished', async () => {
    answer(() => HttpResponse.json({ success: true, data: { envId: 'env-9', pinned: true, started: true } }));
    const user = userEvent.setup();
    const { client } = mount();
    await user.click(await confirm());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const [mutation] = client.getMutationCache().getAll();
    expect(mutation?.options.meta?.success).toBe('Return started. The agent row shows the result.');
  });

  it.each([
    ['updates.manage', 'environments.manage'],
    ['environments.manage', 'updates.manage'],
  ])(
    'stays focusable, is aria-disabled and described by the missing key when only %s is held',
    async (held, missing) => {
      grantPermissions([held]);
      const user = userEvent.setup();
      mount();
      const button = await confirm();
      expect(button).not.toBeDisabled();
      expect(button).toHaveAttribute('aria-disabled', 'true');
      expect(button).toHaveAccessibleDescription(`Missing ${missing}.`);
      expect(screen.getByRole('dialog')).toHaveAccessibleDescription(new RegExp(`Missing ${missing}\\.`));
      button.focus();
      expect(button).toHaveFocus();
      await user.click(button);
      expect(calls).not.toHaveBeenCalled();
    },
  );

  it('names both keys and refuses activation when none is held', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const button = await confirm();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveAccessibleDescription('Missing updates.manage and environments.manage.');
    await user.click(button);
    expect(calls).not.toHaveBeenCalled();
  });

  it('puts the not-connected reason in the dialog description and on Confirm, and sends nothing', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentReturnDialog agent={{ ...agent, connected: false }} open onOpenChange={onOpenChange} />);
    const button = await confirm();
    const reason = 'This agent is not connected. Connect it and try again.';
    expect(screen.getByRole('dialog')).toHaveAccessibleDescription(new RegExp(reason));
    expect(button).toHaveAccessibleDescription(new RegExp(reason));
    expect(button).toHaveAttribute('aria-disabled', 'true');
    await user.click(button);
    expect(calls).not.toHaveBeenCalled();
  });

  it('leaves Confirm enabled and undescribed by a reason for a connected agent', async () => {
    renderWithProviders(<AgentReturnDialog agent={{ ...agent, connected: true }} open onOpenChange={onOpenChange} />);
    const button = await confirm();
    expect(button).not.toHaveAttribute('aria-disabled');
    expect(screen.queryByText('This agent is not connected. Connect it and try again.')).not.toBeInTheDocument();
  });

  it('keeps the dialog open and shows the server message when the move is refused', async () => {
    const message = 'This agent is not connected. Connect it and try again.';
    answer(() => HttpResponse.json({ success: false, error: message, code: 'err.conflict' }, { status: 409 }));
    const user = userEvent.setup();
    mount();
    await user.click(await confirm());
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(await confirm()).toBeEnabled();
  });

  it('sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: {} });
    });
    const user = userEvent.setup();
    mount();
    const button = await confirm();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledTimes(1);
  });
});

describe('AgentStatusCell after a failed return', () => {
  it('leads with the failure, keeps the reason behind the error icon and says it is pinned', async () => {
    const reason = 'agent refused the image: digest mismatch for mcharbor-agent after 3 attempts';
    const user = userEvent.setup();
    renderWithProviders(<AgentStatusCell agent={{ ...agent, pinned: true, returnReason: reason }} excluded />);
    expect(screen.getByText('Return failed, still on 2026.11.02-rc.1')).toBeInTheDocument();
    expect(screen.getByText('Pinned: no updates')).toBeInTheDocument();
    expect(screen.queryByText(reason)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(reason)).toBeInTheDocument();
  });
});
