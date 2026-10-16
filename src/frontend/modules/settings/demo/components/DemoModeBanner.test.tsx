// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DemoModeBanner } from './DemoModeBanner';
import { useDockedBanners } from '@resources/stores/docked-banners';
import { useDemoBannerStore } from '@resources/stores/demoBannerStore';
import type { DemoStatus } from '@resources/types/demo';
import { DEMO_STATUS_SIZES } from '../test-fixtures';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

vi.mock('@resources/stores/mcharbor-restart', () => ({
  runWithMcHarborRestart: vi.fn(async (request: () => Promise<unknown>) => request()),
}));

const active: DemoStatus = {
  available: true,
  active: true,
  databaseExists: true,
  clustered: false,
  canEnter: false,
  enteredAt: '2026-09-27T10:00:00Z',
  ...DEMO_STATUS_SIZES,
};

describe('DemoModeBanner', () => {
  const posts = vi.fn();
  const statusCalls = vi.fn();

  function serve(status: DemoStatus) {
    server.use(
      http.get('*/api/demo/status', () => {
        statusCalls();
        return HttpResponse.json({ success: true, data: status });
      }),
      http.post('*/api/demo/:action', ({ params }) => {
        posts(params.action);
        return HttpResponse.json({ success: true, data: { restartScheduled: false, restartRequired: false } });
      }),
    );
  }

  beforeEach(() => {
    posts.mockReset();
    statusCalls.mockReset();
    grantPermissions(['system.demo']);
  });

  it('renders nothing while demo mode is not active', async () => {
    serve({ ...active, active: false, canEnter: true });
    renderWithProviders(<DemoModeBanner />);
    await waitFor(() => expect(statusCalls).toHaveBeenCalled());
    expect(screen.queryByText('Demo mode')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Exit demo mode' })).not.toBeInTheDocument();
  });

  it('tells every viewer the estate is simulated while demo mode is active', async () => {
    grantPermissions([]);
    serve(active);
    renderWithProviders(<DemoModeBanner />);
    expect(await screen.findByText('Demo mode')).toBeInTheDocument();
    expect(
      screen.getByText('Everything you see is simulated. Actions that would touch real systems are disabled.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Exit demo mode' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled();
  });

  it('exits only after the confirmation, with one request', async () => {
    serve(active);
    const user = userEvent.setup();
    renderWithProviders(<DemoModeBanner />);
    await user.click(await screen.findByRole('button', { name: 'Exit demo mode' }));
    expect(posts).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Exit demo mode' }));
    await waitFor(() => expect(posts).toHaveBeenCalledWith('exit'));
    expect(posts).toHaveBeenCalledTimes(1);
  });

  it('dismisses to the top-bar icon, not a toolbar chip, without leaving demo mode', async () => {
    serve(active);
    const user = userEvent.setup();
    renderWithProviders(<DemoModeBanner />);
    await user.click(await screen.findByRole('button', { name: 'Dismiss' }));
    await waitFor(() => expect(screen.queryByText('Demo mode')).not.toBeInTheDocument());
    expect(posts).not.toHaveBeenCalled();
    expect(useDemoBannerStore.getState().dismissedFor).toBe(active.enteredAt);
    expect(Object.keys(useDockedBanners.getState().docked)).toHaveLength(0);
    useDemoBannerStore.getState().show();
  });
});
