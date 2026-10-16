// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BulkUserActionDialog } from './BulkUserActionDialog';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

type Body = { action: string; userIds: string[]; dryRun: boolean; reason: string };

const preview = {
  action: 'delete',
  dryRun: true,
  items: [
    { userId: 'u1', username: 'alice', outcome: 'would_apply' },
    { userId: 'a1', username: 'root', outcome: 'skipped', reason: 'last_admin' },
  ],
  summary: { total: 2, applied: 1, skipped: 1, failed: 0 },
};

describe('BulkUserActionDialog', () => {
  const applies = vi.fn();
  const previews = vi.fn();
  let applyResponse: () => Response | Promise<Response> = () =>
    HttpResponse.json({ success: true, data: { ...preview, dryRun: false } });

  beforeEach(() => {
    applies.mockReset();
    previews.mockReset();
    grantPermissions(['users.manage']);
    applyResponse = () => HttpResponse.json({ success: true, data: { ...preview, dryRun: false } });
    server.use(
      http.post('*/api/users/bulk/delete', async ({ request }) => {
        const body = (await request.json()) as Body;
        if (body.dryRun) {
          previews(body);
          return HttpResponse.json({ success: true, data: preview });
        }
        applies(body);
        return applyResponse();
      }),
    );
  });

  function mount() {
    const onOpenChange = vi.fn();
    const onDone = vi.fn();
    renderWithProviders(
      <BulkUserActionDialog action="delete" userIds={['u1', 'a1']} onOpenChange={onOpenChange} onDone={onDone} />,
    );
    return { onOpenChange, onDone };
  }

  async function confirm() {
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText('Last active admin');
    return within(dialog).getByRole('button', { name: 'Delete' });
  }

  it('previews as a dry run and applies nothing when cancelled', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await confirm();
    expect(previews).toHaveBeenCalledTimes(1);
    expect(previews.mock.calls[0]![0]).toMatchObject({ dryRun: true, userIds: ['u1', 'a1'] });
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(applies).not.toHaveBeenCalled();
  });

  it('applies exactly once, only after confirmation', async () => {
    const user = userEvent.setup();
    const { onOpenChange, onDone } = mount();
    await user.click(await confirm());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(applies).toHaveBeenCalledTimes(1);
    expect(applies.mock.calls[0]![0]).toMatchObject({ dryRun: false, action: 'delete' });
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('renders the confirm disabled, naming users.manage, when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const button = await confirm();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(applies).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the users.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message when the apply fails', async () => {
    const message = 'bulk delete refused: user u1 owns 4 stacks';
    applyResponse = () => HttpResponse.json({ success: false, error: message, code: 'err.conflict' }, { status: 409 });
    const user = userEvent.setup();
    const { onOpenChange, onDone } = mount();
    await user.click(await confirm());
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it('sends one apply on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    applyResponse = async () => {
      await held;
      return HttpResponse.json({ success: true, data: { ...preview, dryRun: false } });
    };
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    const button = await confirm();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(applies).toHaveBeenCalledTimes(1);
  });
});
