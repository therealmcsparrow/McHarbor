// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactElement } from 'react';
import { screen as rtlScreen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse, type HttpHandler } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from './permissions';
import { renderWithProviders } from './render';
import { server } from './server';

export type DestructiveDialogCase = {
  name: string;
  method: 'post' | 'delete' | 'put';
  path: string;
  permission: string;
  grant?: string[];
  confirmLabel: string;
  handlers?: HttpHandler[];
  body?: unknown;
  reportsSuccess?: boolean;
  render: (props: { onOpenChange: (open: boolean) => void; onSuccess: () => void }) => ReactElement;
};

export function describeDestructiveDialog(c: DestructiveDialogCase) {
  describe(c.name, () => {
    const calls = vi.fn();

    function answer(response: () => Response | Promise<Response>) {
      server.use(
        http[c.method](c.path, async ({ request }) => {
          const text = await request.text();
          calls(text ? JSON.parse(text) : undefined);
          return response();
        }),
      );
    }

    function mount() {
      const onOpenChange = vi.fn();
      const onSuccess = vi.fn();
      renderWithProviders(c.render({ onOpenChange, onSuccess }));
      return { onOpenChange, onSuccess };
    }

    async function confirmButton() {
      const dialog = await rtlScreen.findByRole('dialog');
      return within(dialog).getByRole('button', { name: c.confirmLabel });
    }

    beforeEach(() => {
      calls.mockReset();
      grantPermissions(c.grant ?? [c.permission]);
      if (c.handlers) server.use(...c.handlers);
      answer(() => HttpResponse.json({ success: true, data: {} }));
    });

    it('sends nothing when cancelled', async () => {
      const user = userEvent.setup();
      const { onOpenChange } = mount();
      await user.click(within(await rtlScreen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
      expect(onOpenChange).toHaveBeenCalledWith(false);
      expect(calls).not.toHaveBeenCalled();
    });

    it('sends exactly one request, only after confirmation, then closes', async () => {
      const user = userEvent.setup();
      const { onOpenChange, onSuccess } = mount();
      expect(calls).not.toHaveBeenCalled();
      await user.click(await confirmButton());
      await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
      expect(calls).toHaveBeenCalledTimes(1);
      if (c.body !== undefined) expect(calls).toHaveBeenCalledWith(c.body);
      if (c.reportsSuccess !== false) expect(onSuccess).toHaveBeenCalledTimes(1);
    });

    it(`renders the confirm disabled, naming ${c.permission}, when it is missing`, async () => {
      grantPermissions([]);
      const user = userEvent.setup();
      mount();
      const button = await confirmButton();
      expect(button).toBeDisabled();
      await user.click(button);
      expect(calls).not.toHaveBeenCalled();
      await user.hover(button.parentElement as HTMLElement);
      expect(await rtlScreen.findAllByText(`Requires the ${c.permission} permission`)).not.toHaveLength(0);
    });

    it('keeps the dialog open and shows the server message when the request fails', async () => {
      const message = `${c.name} refused: the target is still in use by 3 containers`;
      answer(() => HttpResponse.json({ success: false, error: message, code: 'err.conflict' }, { status: 409 }));
      const user = userEvent.setup();
      const { onOpenChange, onSuccess } = mount();
      await user.click(await confirmButton());
      expect(await rtlScreen.findByText(message)).toBeInTheDocument();
      expect(onSuccess).not.toHaveBeenCalled();
      expect(onOpenChange).not.toHaveBeenCalledWith(false);
      expect(await confirmButton()).toBeEnabled();
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
      const { onOpenChange } = mount();
      const button = await confirmButton();
      await user.dblClick(button);
      await user.click(button);
      release();
      await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
      expect(calls).toHaveBeenCalledTimes(1);
    });
  });
}
