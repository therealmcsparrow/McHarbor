// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { SuppressedInDemo } from './SuppressedInDemo';
import type { DemoStatus } from '@resources/types/demo';
import { DEMO_STATUS_SIZES } from '../test-fixtures';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

const base: DemoStatus = {
  available: true,
  active: false,
  databaseExists: false,
  clustered: false,
  canEnter: true,
  ...DEMO_STATUS_SIZES,
};

function serve(status: DemoStatus, calls: () => void) {
  server.use(
    http.get('*/api/demo/status', () => {
      calls();
      return HttpResponse.json({ success: true, data: status });
    }),
  );
}

describe('SuppressedInDemo', () => {
  it('shows its content outside demo mode', async () => {
    serve(base, () => undefined);
    renderWithProviders(
      <SuppressedInDemo>
        <p>Licence notice</p>
      </SuppressedInDemo>,
    );
    expect(await screen.findByText('Licence notice')).toBeInTheDocument();
  });

  it('hides its content while demo mode is active', async () => {
    const calls = vi.fn();
    serve({ ...base, active: true, databaseExists: true, canEnter: false, enteredAt: '2026-09-27T10:00:00Z' }, calls);
    renderWithProviders(
      <SuppressedInDemo>
        <p>Licence notice</p>
      </SuppressedInDemo>,
    );
    await waitFor(() => expect(calls).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByText('Licence notice')).not.toBeInTheDocument());
  });
});
