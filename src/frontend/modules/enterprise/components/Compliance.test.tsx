// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { ComplianceReport, ComplianceSection } from '../hooks/useCompliance';
import CompliancePage from '../pages/CompliancePage';
import { ComplianceReportDialog } from './ComplianceReportDialog';

const sections: ComplianceSection[] = [
  {
    id: 'instanceBackup',
    status: 'fail',
    facts: [{ key: 'lastRunStatus', kind: 'code', value: 'failed', sensitive: false }],
    reasons: ['lastBackupFailed'],
  },
  {
    id: 'keys',
    status: 'pass',
    facts: [{ key: 'backupKeyId', kind: 'code', value: 'kid-1', sensitive: true }],
    reasons: [],
  },
  {
    id: 'ha',
    status: 'info',
    facts: [{ key: 'nodeId', kind: 'code', value: 'node-a', sensitive: true }],
    reasons: ['singleNode'],
  },
];

const report: ComplianceReport = {
  generatedAt: '2026-09-26T12:00:00Z',
  version: '2026.09.01',
  build: '202609261200',
  redacted: false,
  summary: { pass: 1, warn: 0, fail: 1, info: 1, unknown: 0 },
  sections,
};

describe('Compliance dashboard', () => {
  const requests = vi.fn();
  beforeEach(() => {
    requests.mockReset();
    grantPermissions(['compliance.view']);
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:x'), revokeObjectURL: vi.fn() }));
  });
  afterEach(() => vi.unstubAllGlobals());

  it('shows each section with its verdict, the reason, and where to fix it', async () => {
    server.use(http.get('*/api/compliance/posture', () => HttpResponse.json({ success: true, data: report })));
    renderWithProviders(
      <MemoryRouter>
        <CompliancePage />
      </MemoryRouter>,
    );
    const card = await screen.findByTestId('compliance-section-instanceBackup');
    expect(within(card).getByText('Fail')).toBeInTheDocument();
    expect(within(card).getByText('The most recent instance backup failed.')).toBeInTheDocument();
    expect(within(card).getByRole('link', { name: /Open/ })).toHaveAttribute('href', '/settings?tab=instance-backup');
    expect(within(screen.getByTestId('compliance-section-keys')).queryByRole('link')).toBeNull();
  });

  it('asks for nothing and names compliance.view without it', async () => {
    grantPermissions([]);
    server.use(
      http.get('*/api/compliance/posture', () => {
        requests();
        return HttpResponse.json({ success: true, data: report });
      }),
    );
    renderWithProviders(
      <MemoryRouter>
        <CompliancePage />
      </MemoryRouter>,
    );
    expect(await screen.findByText('Requires the compliance.view permission')).toBeInTheDocument();
    expect(requests).not.toHaveBeenCalled();
  });

  it('downloads only the picked sections, redacted, with one request on a double click', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get('*/api/compliance/posture', async ({ request }) => {
        requests(Object.fromEntries(new URL(request.url).searchParams));
        await gate;
        return HttpResponse.json({ success: true, data: { ...report, redacted: true } });
      }),
    );
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<ComplianceReportDialog open onOpenChange={onOpenChange} sections={sections} />);
    expect(within(screen.getByTestId('redaction-preview')).getByText('Backup key ID')).toBeInTheDocument();
    await user.click(screen.getByLabelText('High availability'));
    expect(within(screen.getByTestId('redaction-preview')).queryByText('Node ID')).toBeNull();
    await user.dblClick(screen.getByRole('button', { name: /Download$/ }));
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(requests).toHaveBeenCalledTimes(1);
    const params = (requests.mock.calls[0]?.[0] ?? {}) as Record<string, string>;
    const picked = (params.sections ?? '').split(',');
    expect(params.export).toBe('1');
    expect(params.redact).toBe('1');
    expect(picked).not.toContain('ha');
    expect(picked).toContain('keys');
  });

  it('keeps the dialog open with the server message when the report fails', async () => {
    server.use(
      http.get('*/api/compliance/posture', () =>
        HttpResponse.json({ success: false, error: 'database is locked' }, { status: 500 }),
      ),
    );
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<ComplianceReportDialog open onOpenChange={onOpenChange} sections={sections} />);
    await user.click(screen.getByRole('button', { name: /Download$/ }));
    expect(await screen.findByText('database is locked')).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('disables the download and names compliance.view without it', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<ComplianceReportDialog open onOpenChange={vi.fn()} sections={sections} />);
    const download = screen.getByRole('button', { name: /Download$/ });
    expect(download).toBeDisabled();
    await user.hover(download.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the compliance.view permission')).not.toHaveLength(0);
  });
});
