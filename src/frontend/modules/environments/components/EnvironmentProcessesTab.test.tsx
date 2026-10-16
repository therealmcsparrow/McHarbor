// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { EnvironmentProcessesTab } from './EnvironmentProcessesTab';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

describe('EnvironmentProcessesTab', () => {
  it('shows the server message and a working Retry when the stats request fails', async () => {
    let calls = 0;
    server.use(
      http.get('*/api/containers/stats/summary', () => {
        calls += 1;
        return HttpResponse.json({ success: false, error: 'stats collector is down' }, { status: 500 });
      }),
    );
    renderWithProviders(<EnvironmentProcessesTab envId="env-1" />);

    expect(await screen.findByText('stats collector is down')).toBeInTheDocument();
    const before = calls;
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(calls).toBeGreaterThan(before));
  });
});
