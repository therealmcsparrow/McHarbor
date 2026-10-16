// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { UpdateScheduleCard } from './UpdateScheduleCard';
import type { SelfUpdateSettings } from '../hooks/useUpdates';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const settings: SelfUpdateSettings = {
  intervalHours: 24,
  channelIds: [],
  lastSeenVersion: '',
  enabled: true,
  autoInstall: true,
  autoUpdateAgents: false,
  agentExcludedEnvIds: [],
  checkCron: '0 4 * * *',
  installCron: '30 4 * * *',
  installWindowMinutes: 120,
};

const saves = vi.fn();

function mount(state: Record<string, unknown> = {}) {
  server.use(
    http.get('*/api/updates/settings', () => HttpResponse.json({ success: true, data: settings })),
    http.get('*/api/updates/state', () =>
      HttpResponse.json({
        success: true,
        data: {
          currentVersion: '2026.09.01',
          intervalHours: 24,
          nextCheckAt: '2026-09-27T04:00:00Z',
          nextInstallAt: '2026-09-27T04:30:00Z',
          ...state,
        },
      }),
    ),
    http.put('*/api/updates/settings', async ({ request }) => {
      saves(await request.json());
      return HttpResponse.json({ success: true, data: settings });
    }),
  );
  return renderWithProviders(<UpdateScheduleCard />);
}

describe('UpdateScheduleCard', () => {
  beforeEach(() => {
    saves.mockReset();
    grantPermissions(['updates.view', 'updates.manage']);
  });

  it('saves the schedule only when it changed, with the window length', async () => {
    const user = userEvent.setup();
    mount();
    const save = await screen.findByRole('button', { name: 'Save schedule' });
    expect(save).toBeDisabled();
    await user.click(
      within(screen.getByLabelText('Keep the window open for (minutes)').parentElement as HTMLElement).getByRole(
        'button',
        { name: 'Decrease' },
      ),
    );
    await user.click(save);
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves.mock.calls[0]?.[0]).toMatchObject({
      checkCron: '0 4 * * *',
      installCron: '30 4 * * *',
      installWindowMinutes: 119,
      autoInstall: true,
    });
    expect(screen.getByText(/^Next install window:/)).toBeInTheDocument();
  });

  it('says in words why an install is waiting, and when a window was skipped', async () => {
    mount({ installDeferred: 'backup_running' });
    expect(await screen.findByText('Waiting for a container backup or restore to finish')).toBeInTheDocument();
  });

  it('reports a skipped window with its reason', async () => {
    mount({ installDeferred: 'export_running', installSkipped: true });
    expect(
      await screen.findByText(/The last install window closed while waiting: Waiting for an instance export/),
    ).toBeInTheDocument();
  });

  it('renders save disabled, naming the permission, without updates.manage', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    mount();
    const windowField = await screen.findByLabelText('Keep the window open for (minutes)');
    await user.click(within(windowField.parentElement as HTMLElement).getByRole('button', { name: 'Decrease' }));
    const save = screen.getByRole('button', { name: 'Save schedule' });
    expect(save).toBeDisabled();
    await user.hover(save.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the updates.manage permission')).not.toHaveLength(0);
  });
});
