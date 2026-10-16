// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { PackSummary } from '../hooks/useCompliancePacks';
import CompliancePacksPage from '../pages/CompliancePacksPage';

const pack: PackSummary = {
  framework: {
    key: 'cis-docker-benchmark',
    name: 'CIS Docker Benchmark',
    description: 'Evidence for the CIS Docker Benchmark.',
    runnable: true,
    evidence: [{ key: 'docker.info', kind: 'dockerApi', ref: 'GET /info' }],
    controls: [
      {
        id: '2.13',
        group: 'daemon',
        title: 'Live restore is enabled',
        requirement: 'Containers keep running while the daemon is unavailable.',
        coverage: 'mechanical',
        scope: 'daemon',
        evidence: ['docker.info'],
      },
      {
        id: '3.15',
        group: 'files',
        title: 'Docker socket file ownership and permissions are correct',
        requirement: '/var/run/docker.sock is owned by root:docker with mode 660 or stricter.',
        coverage: 'notCovered',
        scope: 'none',
        evidence: ['docker.info'],
      },
    ],
  },
  latestRun: {
    id: 'run-1',
    framework: 'cis-docker-benchmark',
    environmentId: '',
    trigger: 'scheduled',
    startedBy: '',
    startedAt: '2026-09-27T01:00:00Z',
    finishedAt: '2026-09-27T01:02:00Z',
    status: 'failed',
    summary: { pass: 1, warn: 0, fail: 0, info: 1, unknown: 2 },
    lastError: 'the check results could not be recorded: database is locked',
  },
};

const runDetail = {
  run: pack.latestRun,
  results: [
    { id: '2.13', status: 'unknown' as const, facts: [], reasons: ['daemonUnavailable'] },
    { id: '3.15', status: 'info' as const, facts: [], reasons: ['notCoveredByMcHarbor'] },
  ],
};

function servePacks(requests?: () => void) {
  server.use(
    http.get('*/api/compliance/packs', () => {
      requests?.();
      return HttpResponse.json({ success: true, data: [pack] });
    }),
    http.get('*/api/compliance/packs/runs/run-1', () => HttpResponse.json({ success: true, data: runDetail })),
  );
}

function renderPage() {
  return renderWithProviders(
    <MemoryRouter>
      <CompliancePacksPage />
    </MemoryRouter>,
  );
}

describe('Compliance packs', () => {
  beforeEach(() => {
    grantPermissions(['compliance.view']);
  });

  it('never assessed is not a pass: the not-assessed count is called out and the reason is readable', async () => {
    servePacks();
    renderPage();
    expect(await screen.findByText(/2 of this pack's controls carry no verdict/)).toBeInTheDocument();
    expect(screen.getByText(/Not assessed is not a pass/)).toBeInTheDocument();
    const control = await screen.findByTestId('compliance-pack-control-2.13');
    expect(within(control).getByText('Not assessed')).toBeInTheDocument();
    await userEvent.click(within(control).getByRole('button', { name: /Show the facts/ }));
    expect(
      within(control).getByText('The Docker daemon could not be read, so this control has no verdict.'),
    ).toBeInTheDocument();
  });

  it('a control McHarbor cannot see is still listed, and says so', async () => {
    servePacks();
    renderPage();
    const control = await screen.findByTestId('compliance-pack-control-3.15');
    expect(within(control).getByText('Not covered')).toBeInTheDocument();
    await userEvent.click(await within(control).findByRole('button', { name: /Show the facts/ }));
    expect(within(control).getByText(/McHarbor cannot see this control/)).toBeInTheDocument();
  });

  it("a failed run's message reaches the operator in full, behind the icon and never truncated", async () => {
    servePacks();
    renderPage();
    const view = await screen.findByRole('button', { name: 'View the error' });
    await userEvent.click(view);
    expect(await screen.findByText('the check results could not be recorded: database is locked')).toBeInTheDocument();
  });

  it('asks for nothing and names compliance.view without it', async () => {
    grantPermissions([]);
    const requests = vi.fn();
    servePacks(requests);
    renderPage();
    expect(await screen.findByText('Requires the compliance.view permission')).toBeInTheDocument();
    expect(requests).not.toHaveBeenCalled();
  });

  it('disables the run control and names compliance.manage without it', async () => {
    servePacks();
    renderPage();
    const run = await screen.findByRole('button', { name: /Run checks/ });
    expect(run).toBeDisabled();
    await userEvent.hover(run.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the compliance.manage permission')).not.toHaveLength(0);
  });

  it('sends no request without confirmation, then one request on a double click, keeping the dialog open on failure', async () => {
    grantPermissions(['compliance.view', 'compliance.manage']);
    const runs = vi.fn();
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    servePacks();
    server.use(
      http.post('*/api/compliance/packs/cis-docker-benchmark/run', async () => {
        runs();
        await gate;
        return HttpResponse.json({ success: false, error: 'docker daemon is unreachable' }, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Run checks/ }));
    expect(runs).not.toHaveBeenCalled();
    expect(screen.getByTestId('compliance-pack-run-target')).toBeInTheDocument();

    const confirm = within(screen.getByRole('dialog')).getByRole('button', { name: /^Run checks$/ });
    await user.dblClick(confirm);
    release();

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('docker daemon is unreachable'));
    expect(runs).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('no user-facing string in the pack claims compliance or certification', () => {
    const bundle = readFileSync(
      resolve(__dirname, '../../../core/i18n/locales/en/compliancepacks.json'),
      'utf8',
    ).toLowerCase();
    for (const phrase of [
      'compliant with',
      'is compliant',
      'are compliant',
      'certified',
      'we certify',
      'attested by mcharbor',
      'passes cis',
      'guarantees compliance',
      'ensures compliance',
      'audit-ready',
    ]) {
      expect(bundle, `the compliancepacks bundle must never say "${phrase}"`).not.toContain(phrase);
    }
    expect(bundle).toContain('evidence for a framework, not a claim of compliance with one');
  });
});
