// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { PERM } from '@core/auth/permissionKeys';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { useCreateEnvironmentFields } from '../hooks/useCreateEnvironmentFields';
import { useCreateEnvironmentSubmit } from '../hooks/useCreateEnvironmentSubmit';
import { evaluatePrereqGate } from '../hooks/prereqGate';
import type { PrerequisiteReport } from '../hooks/useEnvironmentActions';
import { CreateEnvironmentSshSection } from './CreateEnvironmentSshSection';

const PROBE = '*/api/agents/probe';

function report(overrides: Partial<PrerequisiteReport> = {}): PrerequisiteReport {
  return {
    os: 'linux',
    arch: 'amd64',
    distro: 'debian',
    escalation: 'sudo',
    items: [
      {
        key: 'docker',
        label: 'Docker Engine',
        present: false,
        required: true,
        remediation: 'apt-get install -y docker.io',
      },
    ],
    ...overrides,
  };
}

function Harness() {
  const f = useCreateEnvironmentFields();
  const submit = useCreateEnvironmentSubmit(f, { onOpenChange: () => undefined, onAgentToken: () => undefined });
  const { setSSHHost, setSSHUser, setHostKeyFingerprint, setSSHKey } = f;
  useEffect(() => {
    setSSHHost('10.0.0.5');
    setSSHUser('root');
    setHostKeyFingerprint('SHA256:abc');
    setSSHKey('key');
  }, [setSSHHost, setSSHUser, setHostKeyFingerprint, setSSHKey]);
  return (
    <>
      <CreateEnvironmentSshSection
        f={f}
        probing={submit.probing}
        installing={submit.installing}
        onProbe={submit.handleProbePrereqs}
        onInstall={submit.handleInstallPrereqs}
      />
      <output data-testid="continue">{f.prereqGate.ready ? 'enabled' : 'disabled'}</output>
    </>
  );
}

function probeReplies(data: unknown) {
  server.use(http.post(PROBE, () => HttpResponse.json({ success: true, data })));
}

async function runCheck() {
  const user = userEvent.setup();
  const button = await screen.findByRole('button', { name: /check host/i });
  await waitFor(() => expect(button).toBeEnabled());
  await user.click(button);
  return user;
}

describe('AgentPrerequisitesPanel on the current install flow', () => {
  beforeEach(() => grantPermissions([PERM.ENVIRONMENTS_MANAGE]));

  it('shows the server message when the probe is refused and does not block continue', async () => {
    server.use(
      http.post(PROBE, () =>
        HttpResponse.json({ success: false, error: 'ssh handshake: host key mismatch' }, { status: 400 }),
      ),
    );
    renderWithProviders(<Harness />);
    await runCheck();
    expect(await screen.findByText(/host key mismatch/)).toBeInTheDocument();
    expect(screen.getByTestId('continue')).toHaveTextContent('enabled');
  });

  it('blocks continue while a required package is missing and unblocks once it is ticked for install', async () => {
    probeReplies({ success: true, prerequisites: report() });
    renderWithProviders(<Harness />);
    const user = await runCheck();
    expect(await screen.findByText(/deploy method needs is missing/i)).toBeInTheDocument();
    expect(screen.getByTestId('continue')).toHaveTextContent('disabled');
    await user.click(screen.getByRole('checkbox', { name: 'Docker Engine' }));
    expect(screen.getByTestId('continue')).toHaveTextContent('enabled');
  });

  it('reports ready when nothing required is missing', async () => {
    probeReplies({
      success: true,
      prerequisites: report({ items: [{ key: 'docker', label: 'Docker Engine', present: true, required: true }] }),
    });
    renderWithProviders(<Harness />);
    await runCheck();
    expect(await screen.findByText(/ready to continue/i)).toBeInTheDocument();
    expect(screen.getByTestId('continue')).toHaveTextContent('enabled');
  });

  it('disables the check for a viewer without environments.manage', async () => {
    grantPermissions([]);
    renderWithProviders(<Harness />);
    expect(await screen.findByRole('button', { name: /check host/i })).toBeDisabled();
  });
});

describe('evaluatePrereqGate', () => {
  it('blocks a ticked install when there is no escalation route', () => {
    const gate = evaluatePrereqGate(report({ escalation: 'unavailable' }), ['docker'], '');
    expect(gate.noEscalation).toBe(true);
    expect(gate.ready).toBe(false);
  });

  it('needs the root password when su is the only route', () => {
    const noPassword = report({ escalation: 'unavailable', needsRootPassword: true });
    expect(evaluatePrereqGate(noPassword, ['docker'], '').ready).toBe(false);
    expect(evaluatePrereqGate(noPassword, ['docker'], 'pw').ready).toBe(true);
  });
});
