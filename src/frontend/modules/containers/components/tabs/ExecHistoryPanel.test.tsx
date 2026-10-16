// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ExecHistoryPanel } from './ExecHistoryPanel';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

describe('ExecHistoryPanel', () => {
  const loads = vi.fn();

  beforeEach(() => {
    loads.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
  });

  it('shows the server message when the history cannot be loaded, and retries on request', async () => {
    const user = userEvent.setup();
    server.use(
      http.get('*/api/containers/c1/exec/history', () => {
        loads();
        return HttpResponse.json({ success: false, error: 'Exec history store is offline.' }, { status: 503 });
      }),
    );
    renderWithProviders(<ExecHistoryPanel containerId="c1" />);

    expect(await screen.findByText('Exec history store is offline.')).toBeInTheDocument();
    expect(screen.queryByText(/No exec history/i)).not.toBeInTheDocument();
    expect(loads).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(loads).toHaveBeenCalledTimes(2));
  });
});
