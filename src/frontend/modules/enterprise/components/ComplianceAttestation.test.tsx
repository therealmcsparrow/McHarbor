// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { ComplianceAttestation } from '@resources/hooks/useComplianceAttestations';
import { ComplianceAttestationPanel } from '@resources/components/compliance/ComplianceAttestationPanel';

const previous: ComplianceAttestation = {
  id: 'att-1',
  framework: 'cis-docker-benchmark',
  runId: 'run-0',
  scope: '',
  attestedBy: 'u-ann',
  attestedByName: 'ann',
  attestedAt: '2026-09-01T09:00:00Z',
  note: 'Accepted the Q3 daemon findings; 2.13 is tracked in CHG-42.',
};

const run = { id: 'run-7', startedAt: '2026-09-27T01:00:00Z', status: 'completed' };

function serveLatest(item: ComplianceAttestation | null) {
  server.use(
    http.get('*/api/compliance/attestations', () =>
      HttpResponse.json({
        success: true,
        data: { items: item ? [item] : [], total: item ? 1 : 0, page: 1, per_page: 1, total_pages: 1 },
      }),
    ),
  );
}

function renderPanel() {
  return renderWithProviders(
    <ComplianceAttestationPanel framework="cis-docker-benchmark" frameworkName="CIS Docker Benchmark" run={run} />,
  );
}

describe('Compliance attestation', () => {
  beforeEach(() => {
    grantPermissions(['compliance.view', 'compliance.attest']);
  });

  it('shows the latest sign-off with who, when and the note', async () => {
    serveLatest(previous);
    renderPanel();
    expect(await screen.findByText(/Signed off .* by ann/)).toBeInTheDocument();
    expect(screen.getByText(previous.note)).toBeInTheDocument();
    expect(screen.getByText(/run-0/)).toBeInTheDocument();
  });

  it('disables the sign-off and names compliance.attest without it, even for someone who can run checks', async () => {
    grantPermissions(['compliance.view', 'compliance.manage']);
    serveLatest(null);
    renderPanel();
    const button = await screen.findByRole('button', { name: /Sign off/ });
    expect(button).toBeDisabled();
    await userEvent.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the compliance.attest permission')).not.toHaveLength(0);
  });

  it('sends nothing until confirmed, names the run, and will not confirm without a note', async () => {
    serveLatest(null);
    const posts = vi.fn();
    server.use(
      http.post('*/api/compliance/attestations', () => {
        posts();
        return HttpResponse.json({ success: true, data: previous }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole('button', { name: /Sign off/ }));
    const dialog = screen.getByRole('dialog');
    const target = within(dialog).getByTestId('compliance-attest-target');
    expect(target).toHaveTextContent('CIS Docker Benchmark');
    expect(target).toHaveTextContent('run-7');
    const confirm = within(dialog).getByRole('button', { name: 'Record sign-off' });
    expect(confirm).toBeDisabled();
    await user.type(within(dialog).getByRole('textbox'), '   ');
    expect(confirm).toBeDisabled();
    expect(posts).not.toHaveBeenCalled();
  });

  it('sends one request on a double click and keeps the server message and the dialog on failure', async () => {
    serveLatest(null);
    const posts = vi.fn();
    const bodies: unknown[] = [];
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/compliance/attestations', async ({ request }) => {
        posts();
        bodies.push(await request.json());
        await gate;
        return HttpResponse.json(
          { success: false, error: 'the run belongs to another framework', code: 'err.invalid_body' },
          { status: 400 },
        );
      }),
    );
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole('button', { name: /Sign off/ }));
    const dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByRole('textbox'), '  Reviewed and accepted.  ');
    await user.dblClick(within(dialog).getByRole('button', { name: 'Record sign-off' }));
    release();

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('the run belongs to another framework'));
    expect(posts).toHaveBeenCalledTimes(1);
    expect(bodies[0]).toEqual({ framework: 'cis-docker-benchmark', runId: 'run-7', note: 'Reviewed and accepted.' });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
