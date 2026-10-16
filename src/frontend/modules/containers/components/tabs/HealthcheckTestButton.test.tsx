// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { HealthcheckTestButton } from './HealthcheckTestButton';

const sent = vi.fn();

function answer(result: { exitCode: number; durationMs: number; stdout?: string; stderr?: string }) {
  server.use(
    http.post('*/api/containers/:id/exec', async ({ request }) => {
      sent(await request.json());
      return HttpResponse.json({ success: true, data: { stdout: '', stderr: '', ...result } });
    }),
  );
}

describe('HealthcheckTestButton', () => {
  beforeEach(() => {
    sent.mockReset();
    grantPermissions(['containers.view', 'containers.manage']);
  });

  // Docker runs the editor's command as CMD-SHELL: through /bin/sh -c, once.
  it('runs the command the way Docker will and reports healthy on exit 0', async () => {
    answer({ exitCode: 0, durationMs: 42, stdout: 'ok' });
    const user = userEvent.setup();
    renderWithProviders(
      <HealthcheckTestButton
        containerId="c1"
        running
        command="curl -f http://localhost/ || exit 1"
        timeoutSeconds={5}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Test' }));
    expect(await screen.findByText('Healthy: the command exited 0 in 42 ms.')).toBeInTheDocument();
    expect(sent).toHaveBeenCalledWith({ command: ['curl -f http://localhost/ || exit 1'], shell: '/bin/sh' });
  });

  it('reports unhealthy on any other exit code, and a run past the timeout as a failure', async () => {
    answer({ exitCode: 1, durationMs: 10 });
    const user = userEvent.setup();
    const { unmount } = renderWithProviders(
      <HealthcheckTestButton containerId="c1" running command="false" timeoutSeconds={5} />,
    );
    await user.click(screen.getByRole('button', { name: 'Test' }));
    expect(await screen.findByText('Unhealthy: the command exited 1.')).toBeInTheDocument();
    unmount();

    answer({ exitCode: 0, durationMs: 7000 });
    renderWithProviders(<HealthcheckTestButton containerId="c1" running command="sleep 7" timeoutSeconds={5} />);
    await user.click(screen.getByRole('button', { name: 'Test' }));
    expect(await screen.findByText(/longer than the 5 s timeout/)).toBeInTheDocument();
  });

  it('is disabled while the container is stopped, and says why', () => {
    renderWithProviders(<HealthcheckTestButton containerId="c1" running={false} command="true" timeoutSeconds={5} />);
    expect(screen.getByRole('button', { name: 'Test' })).toBeDisabled();
    expect(screen.getByText('Start the container to test its healthcheck.')).toBeInTheDocument();
  });

  it('is disabled and names containers.manage when the permission is missing', async () => {
    grantPermissions(['containers.view']);
    const user = userEvent.setup();
    renderWithProviders(<HealthcheckTestButton containerId="c1" running command="true" timeoutSeconds={5} />);
    const button = screen.getByRole('button', { name: /Test/ });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement ?? button);
    expect((await screen.findAllByText(/containers\.manage/)).length).toBeGreaterThan(0);
    expect(sent).not.toHaveBeenCalled();
  });
});
