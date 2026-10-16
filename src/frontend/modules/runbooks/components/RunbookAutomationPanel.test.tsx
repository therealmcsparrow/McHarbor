// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { RunbookAutomationPanel } from './RunbookAutomationPanel';
import type { Runbook } from '@resources/types/runbooks';

const runbook: Runbook = {
  id: 'rb-1',
  name: 'Restart loop',
  description: '',
  attachKind: 'alert_condition',
  attachSourceType: 'container',
  attachConditionKey: 'restarts',
  enabled: true,
  createdAt: '2026-09-27T03:00:00Z',
  updatedAt: '2026-09-27T03:00:00Z',
  steps: [
    {
      id: 'st-1',
      runbookId: 'rb-1',
      position: 1,
      kind: 'diagnostic',
      title: 'Look at it',
      diagnosticKey: 'container.state',
      required: true,
      requiredPermissions: ['containers.view'],
    },
  ],
  automation: {
    principalId: 'svc-1',
    principalName: 'runbook-bot',
    onAlert: true,
    lastRunAt: '2026-09-27T03:10:00Z',
    lastError: 'stopped at step 1 (Look at it): the service principal runbook-bot does not hold containers.view',
  },
};

function principals() {
  server.use(
    http.get('*/api/runbooks/principals', () =>
      HttpResponse.json({ success: true, data: [{ id: 'svc-1', username: 'runbook-bot', active: true }] }),
    ),
  );
}

describe('RunbookAutomationPanel', () => {
  it('renders Save disabled without runbooks.automate, names the key, and sends nothing', async () => {
    const calls = vi.fn();
    server.use(
      http.put('*/api/runbooks/rb-1/automation', () => {
        calls();
        return HttpResponse.json({ success: true, data: runbook });
      }),
    );
    grantPermissions(['runbooks.view', 'runbooks.manage']);
    const user = userEvent.setup();
    renderWithProviders(<RunbookAutomationPanel runbook={runbook} />);

    const save = await screen.findByRole('button', { name: 'Save unattended runs' });
    expect(save).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Turn off' })).toBeDisabled();
    await user.click(save);
    expect(calls).not.toHaveBeenCalled();
    await user.hover(save.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the runbooks.automate permission')).not.toHaveLength(0);
  });

  it('sends the principal and triggers as chosen, and Turn off clears them', async () => {
    const bodies: unknown[] = [];
    principals();
    server.use(
      http.put('*/api/runbooks/rb-1/automation', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: runbook });
      }),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookAutomationPanel runbook={runbook} />);

    await user.click(await screen.findByRole('button', { name: 'Save unattended runs' }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    await user.click(screen.getByRole('button', { name: 'Turn off' }));
    await waitFor(() => expect(bodies).toHaveLength(2));
    expect(bodies[0]).toEqual({ principalId: 'svc-1', onAlert: true, cron: '' });
    expect(bodies[1]).toEqual({ principalId: '', onAlert: false, cron: '' });
  });

  it("shows the server's refusal intact", async () => {
    const message = 'That schedule is not a valid five-field cron expression';
    principals();
    server.use(
      http.put('*/api/runbooks/rb-1/automation', () =>
        HttpResponse.json({ success: false, error: message, code: 'err.runbook.cron_invalid' }, { status: 400 }),
      ),
    );
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookAutomationPanel runbook={runbook} />);

    await user.click(await screen.findByRole('button', { name: 'Save unattended runs' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
  });

  it('refuses unattended mode when the first step is an instruction', async () => {
    principals();
    grantPermissions('*');
    renderWithProviders(
      <RunbookAutomationPanel
        runbook={{ ...runbook, steps: [{ ...runbook.steps[0]!, kind: 'instruction', diagnosticKey: undefined }] }}
      />,
    );

    expect(await screen.findByRole('button', { name: 'Save unattended runs' })).toBeDisabled();
    expect(
      screen.getByText(
        'The first step is an instruction for a person, so this runbook cannot run unattended. Put a diagnostic or an action first.',
      ),
    ).toBeInTheDocument();
  });

  it("keeps the last run's stop reason whole behind the error icon", async () => {
    principals();
    grantPermissions('*');
    const user = userEvent.setup();
    renderWithProviders(<RunbookAutomationPanel runbook={runbook} />);

    await user.click(await screen.findByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(runbook.automation!.lastError!)).toBeInTheDocument();
  });
});
