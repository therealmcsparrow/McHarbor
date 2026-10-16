// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DemoModeHeaderIcon } from '@resources/components/demo/DemoModeHeaderIcon';
import { useDemoBannerStore } from '@resources/stores/demoBannerStore';
import type { DemoStatus } from '@resources/types/demo';
import { DEMO_STATUS_SIZES } from '../test-fixtures';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

const active: DemoStatus = {
  available: true,
  active: true,
  databaseExists: true,
  clustered: false,
  canEnter: false,
  enteredAt: '2026-09-27T10:00:00Z',
  ...DEMO_STATUS_SIZES,
};

function serve(status: DemoStatus, calls: () => void = () => undefined) {
  server.use(
    http.get('*/api/demo/status', () => {
      calls();
      return HttpResponse.json({ success: true, data: status });
    }),
  );
}

describe('DemoModeHeaderIcon', () => {
  afterEach(() => useDemoBannerStore.getState().show());

  it('stays hidden while the banner is on screen', async () => {
    const calls = vi.fn();
    serve(active, calls);
    renderWithProviders(<DemoModeHeaderIcon />);
    await waitFor(() => expect(calls).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: 'Show the demo mode banner' })).not.toBeInTheDocument();
  });

  it('appears once the banner is dismissed and brings it back on click', async () => {
    useDemoBannerStore.getState().dismiss(active.enteredAt ?? '');
    serve(active);
    const user = userEvent.setup();
    renderWithProviders(<DemoModeHeaderIcon />);
    await user.click(await screen.findByRole('button', { name: 'Show the demo mode banner' }));
    expect(useDemoBannerStore.getState().dismissedFor).toBeNull();
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Show the demo mode banner' })).not.toBeInTheDocument(),
    );
  });

  it('does not appear outside demo mode even if an old visit was dismissed', async () => {
    useDemoBannerStore.getState().dismiss(active.enteredAt ?? '');
    const calls = vi.fn();
    serve({ ...active, active: false, canEnter: true, enteredAt: undefined }, calls);
    renderWithProviders(<DemoModeHeaderIcon />);
    await waitFor(() => expect(calls).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: 'Show the demo mode banner' })).not.toBeInTheDocument();
  });
});
