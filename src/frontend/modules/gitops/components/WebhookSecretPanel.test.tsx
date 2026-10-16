// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { WebhookSecretPanel } from './WebhookSecretPanel';

const SECRET = 'whsec_0123456789abcdef0123456789abcdef';
const URL = '*/api/gitops/repos/repo-1/webhook-secret';

describe('WebhookSecretPanel', () => {
  const removeCalls = vi.fn();
  const rotateCalls = vi.fn();
  let configured = true;

  function answerRemove(response: () => Response | Promise<Response>) {
    server.use(
      http.delete(URL, () => {
        removeCalls();
        return response();
      }),
    );
  }

  function answerRotate(response: () => Response | Promise<Response>) {
    server.use(
      http.post(URL, () => {
        rotateCalls();
        return response();
      }),
    );
  }

  const mount = () => renderWithProviders(<WebhookSecretPanel repos={[{ id: 'repo-1', name: 'web' }]} />);

  const removeButton = () => screen.findByRole('button', { name: 'Remove webhook secret for web' });
  const dialog = async () => within(await screen.findByRole('dialog'));

  beforeEach(() => {
    removeCalls.mockReset();
    rotateCalls.mockReset();
    configured = true;
    grantPermissions(['gitops.view', 'gitops.manage']);
    server.use(http.get(URL, () => HttpResponse.json({ success: true, data: { repoId: 'repo-1', configured } })));
    answerRemove(() => new HttpResponse(null, { status: 204 }));
    answerRotate(() =>
      HttpResponse.json({ success: true, data: { repoId: 'repo-1', secret: SECRET } }, { status: 201 }),
    );
  });

  it('sends nothing to remove until the dialog is confirmed', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await removeButton());
    expect(await dialog()).toBeTruthy();
    expect(removeCalls).not.toHaveBeenCalled();
    await user.click((await dialog()).getByRole('button', { name: 'Cancel' }));
    expect(removeCalls).not.toHaveBeenCalled();
  });

  it('renders remove disabled, naming gitops.manage, when the permission is missing', async () => {
    grantPermissions(['gitops.view']);
    const user = userEvent.setup();
    mount();
    const button = await removeButton();
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    await user.hover(button);
    expect(await screen.findAllByText(/gitops\.manage/)).not.toHaveLength(0);
    await user.click(button);
    expect(removeCalls).not.toHaveBeenCalled();
  });

  it("shows the server's own message intact and keeps the dialog open", async () => {
    const message = 'the repository store is read-only, nothing was changed';
    answerRemove(() => HttpResponse.json({ success: false, error: message }, { status: 500 }));
    const user = userEvent.setup();
    mount();
    await user.click(await removeButton());
    await user.click((await dialog()).getByRole('button', { name: 'Remove' }));
    const alerts = await screen.findAllByRole('alert');
    expect(alerts.map((node) => node.textContent ?? '').join(' ')).toContain(message);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends exactly one request on a double click with the response held open', async () => {
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answerRemove(async () => {
      await held;
      return new HttpResponse(null, { status: 204 });
    });
    const user = userEvent.setup();
    mount();
    await user.click(await removeButton());
    const confirm = (await dialog()).getByRole('button', { name: 'Remove' });
    await user.click(confirm);
    await user.click(confirm);
    expect(removeCalls).toHaveBeenCalledTimes(1);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(removeCalls).toHaveBeenCalledTimes(1);
  });

  it('shows a rotated secret once, masked by default, and never again', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole('button', { name: 'Rotate webhook secret for web' }));
    expect(rotateCalls).not.toHaveBeenCalled();
    await user.click((await dialog()).getByRole('button', { name: 'Rotate' }));

    await waitFor(() => expect(rotateCalls).toHaveBeenCalledTimes(1));
    const reveal = await dialog();
    await reveal.findByText('This is the only time it is shown');
    const field = reveal.getByDisplayValue(SECRET);
    expect(field).toHaveAttribute('type', 'password');
    await user.click(reveal.getByRole('button', { name: 'Show secret' }));
    expect(field).toHaveAttribute('type', 'text');

    await user.click(reveal.getAllByRole('button', { name: 'Close' }).at(-1)!);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.queryByDisplayValue(SECRET)).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain(SECRET);
  });

  it('generates without a confirmation when no secret exists yet', async () => {
    configured = false;
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole('button', { name: 'Generate webhook secret for web' }));
    expect(await screen.findByDisplayValue(SECRET)).toBeInTheDocument();
    expect(rotateCalls).toHaveBeenCalledTimes(1);
  });
});
