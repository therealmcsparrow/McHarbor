// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { describeDestructiveDialog } from '../../../test/destructive';
import { RunbookActionDialog } from '@resources/components/runbooks/RunbookActionDialog';
import { RunbookRunner } from '@resources/components/runbooks/RunbookRunner';
import { firstOpenStep, stoppedAtPosition, type Runbook, type RunbookRun } from '@resources/types/runbooks';

const actionStep = {
  id: 'st-2',
  runbookId: 'rb-1',
  position: 2,
  kind: 'action' as const,
  title: 'Restart the container',
  body: 'Only if the log shows the OOM kill.',
  actionKey: 'container.restart',
  actionParams: {},
  required: true,
  requiredPermissions: ['containers.manage'],
};

const instructionStep = {
  id: 'st-1',
  runbookId: 'rb-1',
  position: 1,
  kind: 'instruction' as const,
  title: 'Check the dashboard',
  body: '',
  required: true,
};

const runbook: Runbook = {
  id: 'rb-1',
  name: 'Restart loop',
  description: 'What to do when a container will not stay up',
  attachKind: 'alert_condition',
  attachSourceType: 'container',
  attachConditionKey: 'restarts',
  enabled: true,
  createdAt: '2026-09-26T03:00:00Z',
  updatedAt: '2026-09-26T03:00:00Z',
  steps: [instructionStep, actionStep],
};

const run: RunbookRun = {
  id: 'run-1',
  runbookId: 'rb-1',
  runbookName: 'Restart loop',
  environmentId: 'env-1',
  subjectKind: 'container',
  subjectName: 'web',
  startedBy: 'u-ops',
  startedByName: 'ops',
  startedAt: '2026-09-26T03:01:00Z',
  outcome: 'in_progress',
  results: [
    {
      id: 'res-1',
      runId: 'run-1',
      stepId: 'st-1',
      stepPosition: 1,
      stepTitle: 'Check the dashboard',
      stepKind: 'instruction',
      state: 'done',
      actedBy: 'u-ops',
      actedByName: 'ops',
      at: '2026-09-26T03:02:00Z',
    },
  ],
};

function mount() {
  return renderWithProviders(
    <MemoryRouter>
      <RunbookRunner runbook={runbook} run={run} />
    </MemoryRouter>,
  );
}

describe('Runbook action step authorisation', () => {
  it('renders the action disabled, naming the step permission, and sends nothing', async () => {
    const calls = vi.fn();
    server.use(
      http.post('*/api/runbooks/runs/run-1/steps/st-2', () => {
        calls();
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
    grantPermissions(['runbooks.run', 'runbooks.view']);
    const user = userEvent.setup();
    mount();

    const button = await screen.findByRole('button', { name: 'Run this step' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(calls).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the containers.manage permission')).not.toHaveLength(0);
  });

  it('names the permission the server refused rather than a bare failure', async () => {
    server.use(
      http.post('*/api/runbooks/runs/run-1/steps/st-2', () =>
        HttpResponse.json(
          {
            success: false,
            error: 'You do not hold the permission this step needs',
            code: 'err.runbook.step_forbidden',
            data: { permission: 'containers.manage' },
          },
          { status: 403 },
        ),
      ),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByRole('button', { name: 'Run this step' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Run this step' }));
    expect(await screen.findByText('Requires the containers.manage permission')).toBeInTheDocument();
  });

  it("keeps a failed step's message intact and reachable behind the error icon", async () => {
    const message = 'cannot restart container web: tried to kill container, but did not receive an exit event';
    const failedRun: RunbookRun = {
      ...run,
      results: [
        ...run.results,
        {
          id: 'res-2',
          runId: 'run-1',
          stepId: 'st-2',
          stepPosition: 2,
          stepTitle: 'Restart the container',
          stepKind: 'action',
          state: 'failed',
          lastError: message,
          actedBy: 'u-ops',
          actedByName: 'ops',
          at: '2026-09-26T03:03:00Z',
        },
      ],
    };
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(
      <MemoryRouter>
        <RunbookRunner runbook={runbook} run={failedRun} />
      </MemoryRouter>,
    );

    expect(screen.queryByText(message)).not.toBeInTheDocument();
    await user.click(await screen.findByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(message)).toBeInTheDocument();
  });

  it('refuses to record a step while offline instead of queueing it', async () => {
    const calls = vi.fn();
    server.use(
      http.post('*/api/runbooks/runs/run-1/steps/st-2', () => {
        calls();
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
    grantPermissions('*');
    const onLine = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
    const user = userEvent.setup();
    mount();

    expect(
      await screen.findByText(/You are offline\. You can read this runbook, but a step cannot be recorded/),
    ).toBeInTheDocument();
    const button = await screen.findByRole('button', { name: 'Run this step' });
    expect(button).toBeDisabled();
    await user.click(button);
    await waitFor(() => expect(calls).not.toHaveBeenCalled());
    onLine.mockRestore();
  });
});

describeDestructiveDialog({
  name: 'Runbook action step',
  method: 'post',
  path: '*/api/runbooks/runs/run-1/steps/st-2',
  permission: 'containers.manage',
  grant: ['runbooks.run', 'containers.manage'],
  confirmLabel: 'Run this step',
  body: { state: 'done', skipReason: '', finding: 'the log showed an OOM kill' },
  reportsSuccess: false,
  render: ({ onOpenChange }) => (
    <RunbookActionDialog
      runId="run-1"
      step={actionStep}
      finding="the log showed an OOM kill"
      open
      onOpenChange={onOpenChange}
    />
  ),
});

describe('Runbook run pure logic', () => {
  it('resumes at the first step with no result', () => {
    expect(firstOpenStep(run, runbook.steps)).toBe(1);
  });

  it('resumes at a failed step rather than walking past it', () => {
    const failed: RunbookRun = {
      ...run,
      results: [{ ...run.results[0]!, state: 'failed', lastError: 'nope' }],
    };
    expect(firstOpenStep(failed, runbook.steps)).toBe(0);
  });

  it('reports the furthest step the operator reached', () => {
    expect(stoppedAtPosition(run)).toBe(1);
  });
});
