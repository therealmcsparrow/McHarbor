// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse, type JsonBodyType } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';
import { appToast } from '@resources/utils/toast';
import { DemoActionDialog } from './DemoActionDialog';
import { describeDestructiveDialog } from '../../../../test/destructive';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

vi.mock('@resources/stores/mcharbor-restart', () => ({
  runWithMcHarborRestart: vi.fn(async (request: () => Promise<unknown>) => request()),
}));

vi.mock('@resources/utils/toast', () => ({
  appToast: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

describeDestructiveDialog({
  name: 'DemoActionDialog enter',
  method: 'post',
  path: '*/api/demo/enter',
  permission: 'system.demo',
  confirmLabel: 'Enter demo mode',
  render: ({ onOpenChange, onSuccess }) => (
    <DemoActionDialog action="enter" active={false} open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describeDestructiveDialog({
  name: 'DemoActionDialog exit',
  method: 'post',
  path: '*/api/demo/exit',
  permission: 'system.demo',
  confirmLabel: 'Exit demo mode',
  render: ({ onOpenChange, onSuccess }) => (
    <DemoActionDialog action="exit" active open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describeDestructiveDialog({
  name: 'DemoActionDialog reset',
  method: 'post',
  path: '*/api/demo/reset',
  permission: 'system.demo',
  confirmLabel: 'Reset demo data',
  render: ({ onOpenChange, onSuccess }) => (
    <DemoActionDialog action="reset" active open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describe('DemoActionDialog outcomes', () => {
  const restart = vi.mocked(runWithMcHarborRestart);
  const toast = vi.mocked(appToast);

  beforeEach(() => {
    restart.mockClear();
    toast.success.mockClear();
    toast.warning.mockClear();
    grantPermissions(['system.demo']);
  });

  function answer(path: string, body: JsonBodyType, status = 200) {
    const calls = vi.fn();
    server.use(
      http.post(path, () => {
        calls();
        return HttpResponse.json(body, { status });
      }),
    );
    return calls;
  }

  async function confirm(label: string) {
    const user = userEvent.setup();
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: label }));
  }

  it('translates a refusal reason and keeps the dialog open', async () => {
    const calls = answer(
      '*/api/demo/enter',
      { success: false, error: 'Forbidden', code: 'err.forbidden', data: { reason: 'busy' } },
      400,
    );
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoActionDialog action="enter" active={false} open onOpenChange={onOpenChange} />);
    await confirm('Enter demo mode');
    expect(
      await screen.findByText('Another demo mode change is already in progress. Try again in a moment.'),
    ).toBeInTheDocument();
    expect(calls).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(restart).not.toHaveBeenCalled();
  });

  it('hands a scheduled restart to the restart overlay', async () => {
    answer('*/api/demo/enter', { success: true, data: { restartScheduled: true, restartRequired: false } });
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoActionDialog action="enter" active={false} open onOpenChange={onOpenChange} />);
    await confirm('Enter demo mode');
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(restart).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith('Entering demo mode. McHarbor is restarting.');
  });

  it('asks for a manual restart when none could be scheduled', async () => {
    answer('*/api/demo/exit', { success: true, data: { restartScheduled: false, restartRequired: true } });
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoActionDialog action="exit" active open onOpenChange={onOpenChange} />);
    await confirm('Exit demo mode');
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(restart).not.toHaveBeenCalled();
    expect(toast.warning).toHaveBeenCalledWith(
      'McHarbor could not restart itself. Restart it manually to apply the change.',
    );
  });

  it('deletes the demo data without a restart when demo mode is not active', async () => {
    answer('*/api/demo/reset', { success: true, data: { restartScheduled: false, restartRequired: false } });
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoActionDialog action="reset" active={false} open onOpenChange={onOpenChange} />);
    expect(
      screen.getByText(
        'The demo database is deleted, and the next visit starts from a freshly generated estate. Your real data is not affected.',
      ),
    ).toBeInTheDocument();
    await confirm('Reset demo data');
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(restart).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith('Demo data deleted.');
  });
});
