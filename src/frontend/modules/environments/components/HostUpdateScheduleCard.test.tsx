// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostUpdateScheduleCard } from '@resources/components/host/HostUpdateScheduleCard';
import type { HostTarget } from '@resources/hooks/useHostUpdates';

const ENV_TARGET: HostTarget = { kind: 'environment', envId: 'env-1' };

const schedule = {
  enabled: true,
  time: '03:00',
  days: ['mon'],
  withNewPackages: false,
  skipUnavailable: false,
  removeUnused: false,
};

describe('HostUpdateScheduleCard', () => {
  const calls = vi.fn();

  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['host_updates.manage']);
    server.use(
      http.put('*/api/system/os-updates/schedule', async ({ request }) => {
        calls(await request.json());
        return HttpResponse.json({ success: true, data: null });
      }),
    );
  });

  it('saves Skip unavailable packages with the schedule', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostUpdateScheduleCard target={ENV_TARGET} schedule={schedule} />);
    await user.click(screen.getByRole('switch', { name: 'Skip unavailable packages' }));
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(1));
    expect(calls).toHaveBeenCalledWith({ ...schedule, skipUnavailable: true });
  });

  it('saves Remove unused packages with the schedule', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostUpdateScheduleCard target={ENV_TARGET} schedule={schedule} />);
    await user.click(screen.getByRole('switch', { name: 'Remove unused packages' }));
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(1));
    expect(calls).toHaveBeenCalledWith({ ...schedule, removeUnused: true });
  });

  it('shows the stored value when the schedule loads', async () => {
    renderWithProviders(
      <HostUpdateScheduleCard target={ENV_TARGET} schedule={{ ...schedule, skipUnavailable: true }} />,
    );
    expect(await screen.findByRole('switch', { name: 'Skip unavailable packages' })).toBeChecked();
    expect(screen.getByRole('switch', { name: 'Remove unused packages' })).not.toBeChecked();
  });
});
