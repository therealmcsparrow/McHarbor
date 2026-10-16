// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse, type JsonBodyType } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DemoSizesDialog } from './DemoSizesDialog';
import { DEMO_DEFAULTS, demoSizes } from '../test-fixtures';
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

const parked = { ...DEMO_DEFAULTS, environments: 4, swarmClusters: 1, swarmNodes: 3, historyDays: 30 };

describeDestructiveDialog({
  name: 'DemoSizesDialog enter',
  method: 'post',
  path: '*/api/demo/enter',
  permission: 'system.demo',
  confirmLabel: 'Enter demo mode',
  body: { config: DEMO_DEFAULTS },
  render: ({ onOpenChange, onSuccess }) => (
    <DemoSizesDialog mode="enter" sizes={demoSizes()} open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describeDestructiveDialog({
  name: 'DemoSizesDialog apply new sizes',
  method: 'post',
  path: '*/api/demo/reset',
  permission: 'system.demo',
  confirmLabel: 'Apply new sizes',
  body: { config: parked },
  render: ({ onOpenChange, onSuccess }) => (
    <DemoSizesDialog mode="resize" sizes={demoSizes(parked)} open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describe('DemoSizesDialog form', () => {
  const calls = vi.fn();

  function answer(path: string, body: JsonBodyType, status = 200) {
    server.use(
      http.post(path, async ({ request }) => {
        const text = await request.text();
        calls(text ? JSON.parse(text) : undefined);
        return HttpResponse.json(body, { status });
      }),
    );
  }

  const ok = { success: true, data: { restartScheduled: false, restartRequired: false } };

  function confirmButton(label: string) {
    return within(screen.getByRole('dialog')).getByRole('button', { name: label });
  }

  async function replace(label: string, value: string) {
    const user = userEvent.setup();
    const input = screen.getByLabelText(label);
    await user.clear(input);
    await user.type(input, value);
    return input;
  }

  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['system.demo']);
  });

  it('prefills from the configured sizes and shows each default as a hint', async () => {
    renderWithProviders(<DemoSizesDialog mode="resize" sizes={demoSizes(parked)} open onOpenChange={vi.fn()} />);
    await screen.findByRole('dialog');
    expect(screen.getByLabelText('Environments')).toHaveValue(4);
    expect(screen.getByLabelText('History period')).toHaveValue(30);
    expect(screen.getByLabelText('Swarm services')).toHaveValue(60);
    expect(screen.getByText('Default: 16.')).toBeInTheDocument();
    expect(screen.getByText('Swarm')).toBeInTheDocument();
  });

  it('restores the defaults', async () => {
    const user = userEvent.setup();
    renderWithProviders(<DemoSizesDialog mode="resize" sizes={demoSizes(parked)} open onOpenChange={vi.fn()} />);
    await screen.findByRole('dialog');
    await user.click(screen.getByRole('button', { name: 'Restore defaults' }));
    expect(screen.getByLabelText('Environments')).toHaveValue(16);
    expect(screen.getByLabelText('History period')).toHaveValue(90);
    expect(screen.getByLabelText('Swarm nodes')).toHaveValue(9);
    expect(screen.getByRole('button', { name: 'Restore defaults' })).toBeDisabled();
  });

  it('blocks submit with the range named when a size is out of bounds', async () => {
    answer('*/api/demo/enter', ok);
    const user = userEvent.setup();
    renderWithProviders(<DemoSizesDialog mode="enter" sizes={demoSizes()} open onOpenChange={vi.fn()} />);
    await screen.findByRole('dialog');
    const input = await replace('Environments', '49');
    expect(screen.getByText('Enter a whole number from 1 to 48.')).toBeInTheDocument();
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(confirmButton('Enter demo mode')).toBeDisabled();
    await user.click(confirmButton('Enter demo mode'));
    expect(calls).not.toHaveBeenCalled();
  });

  it('blocks submit when there are fewer Swarm nodes than Swarm clusters', async () => {
    answer('*/api/demo/enter', ok);
    renderWithProviders(<DemoSizesDialog mode="enter" sizes={demoSizes()} open onOpenChange={vi.fn()} />);
    await screen.findByRole('dialog');
    await replace('Swarm clusters', '4');
    await replace('Swarm nodes', '3');
    expect(screen.getByText('Swarm needs at least one node per cluster: enter 4 or more.')).toBeInTheDocument();
    expect(confirmButton('Enter demo mode')).toBeDisabled();
    await replace('Swarm nodes', '4');
    expect(confirmButton('Enter demo mode')).toBeEnabled();
  });

  it('sends the edited sizes when entering', async () => {
    answer('*/api/demo/enter', ok);
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoSizesDialog mode="enter" sizes={demoSizes()} open onOpenChange={onOpenChange} />);
    await screen.findByRole('dialog');
    await replace('Environments', '8');
    await replace('History period', '30');
    await replace('Swarm clusters', '0');
    await user.click(confirmButton('Enter demo mode'));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledTimes(1);
    expect(calls).toHaveBeenCalledWith({
      config: { ...DEMO_DEFAULTS, environments: 8, historyDays: 30, swarmClusters: 0 },
    });
  });

  it('puts a server invalid_config refusal on its field and keeps the dialog open', async () => {
    answer(
      '*/api/demo/reset',
      {
        success: false,
        error: 'swarmServices must be between 0 and 100',
        code: 'err.invalid_body',
        data: { reason: 'invalid_config', field: 'swarmServices', min: 0, max: 100 },
      },
      400,
    );
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<DemoSizesDialog mode="resize" sizes={demoSizes()} open onOpenChange={onOpenChange} />);
    await screen.findByRole('dialog');
    await replace('Swarm services', '300');
    await user.click(confirmButton('Apply new sizes'));
    expect(await screen.findByText('Enter a whole number from 0 to 100.')).toBeInTheDocument();
    expect(screen.getByLabelText('Swarm services')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByRole('alert')).toHaveTextContent(
      'McHarbor refused these sizes. Correct the highlighted field and try again.',
    );
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    await replace('Swarm services', '90');
    expect(screen.queryByText('Enter a whole number from 0 to 100.')).not.toBeInTheDocument();
  });
});
