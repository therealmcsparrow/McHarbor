// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { AgentUpdateSettingsCard } from './AgentUpdateSettingsCard';

const BOTH = ['updates.manage', 'environments.manage'];
const deletes = vi.fn();

function serve(onDelete: () => Response | Promise<Response>) {
  server.use(
    http.get('*/api/updates/settings', () =>
      HttpResponse.json({
        success: true,
        data: { enabled: true, autoInstall: false, autoUpdateAgents: false, agentExcludedEnvIds: ['env-9'] },
      }),
    ),
    http.get('*/api/updates/state', () => HttpResponse.json({ success: true, data: {} })),
    http.get('*/api/updates/agents', () =>
      HttpResponse.json({
        success: true,
        data: {
          agents: [{ envId: 'env-9', name: 'homelab01', version: '2026.11.01', pinned: true, updatable: true }],
          targetVersion: '2026.11.01',
          excludedEnvIds: ['env-9'],
        },
      }),
    ),
    http.delete('*/api/updates/agents/env-9/pin', () => {
      deletes();
      return onDelete();
    }),
  );
}

async function openRelease() {
  const user = userEvent.setup();
  renderWithProviders(<AgentUpdateSettingsCard />);
  const trigger = await screen.findByRole('button', { name: 'Release pin for homelab01' });
  return { user, trigger };
}

async function dialogConfirm() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).getByRole('button', { name: 'Release pin' });
}

beforeEach(() => {
  deletes.mockReset();
  grantPermissions(BOTH);
  serve(() => HttpResponse.json({ success: true, data: { envId: 'env-9', pinned: false } }));
});

describe('release pin', () => {
  it('sends nothing until the operator confirms, and nothing after Cancel', async () => {
    const { user, trigger } = await openRelease();
    await user.click(trigger);
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(deletes).not.toHaveBeenCalled();
    await user.click(trigger);
    await user.click(await dialogConfirm());
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
  });

  it.each([
    ['updates.manage', 'environments.manage'],
    ['environments.manage', 'updates.manage'],
  ])('is disabled and names the missing key when only %s is held', async (held, missing) => {
    grantPermissions([held]);
    const { user, trigger } = await openRelease();
    expect(trigger).toBeDisabled();
    expect(await screen.findByText(`Missing ${missing}.`)).toBeInTheDocument();
    await user.click(trigger);
    expect(deletes).not.toHaveBeenCalled();
  });

  it('keeps the dialog open and shows the server message when the release is refused', async () => {
    const message = 'The pin could not be released: settings are locked by another change';
    serve(() => HttpResponse.json({ success: false, error: message, code: 'err.conflict' }, { status: 409 }));
    const { user, trigger } = await openRelease();
    await user.click(trigger);
    await user.click(await dialogConfirm());
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(await dialogConfirm()).toBeEnabled();
  });

  it('sends one request on a double click, with the response held open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    serve(async () => {
      await held;
      return HttpResponse.json({ success: true, data: { envId: 'env-9', pinned: false } });
    });
    const { user, trigger } = await openRelease();
    await user.click(trigger);
    const button = await dialogConfirm();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(deletes).toHaveBeenCalledTimes(1);
  });
});
