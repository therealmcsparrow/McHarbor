// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuditCoveragePanel } from './AuditCoveragePanel';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const coverage = {
  totalRoutes: 12,
  requiredRoutes: 10,
  mappedRoutes: 9,
  unmappedRequired: 1,
  coveragePercent: 90,
  routes: [],
};

const breakdown = { info: 1, success: 1, warning: 0, error: 0 };

const plugins = [
  {
    pluginSlug: 'sample-tool',
    count: 7,
    lastSeen: '2026-08-25T09:02:00Z',
    installed: true,
    enabled: true,
    severityBreakdown: breakdown,
  },
  {
    pluginSlug: 'gone-plugin',
    count: 2,
    lastSeen: '2026-08-24T09:05:00Z',
    installed: false,
    enabled: false,
    severityBreakdown: breakdown,
  },
];

function renderPanel() {
  return renderWithProviders(
    <MemoryRouter>
      <AuditCoveragePanel />
    </MemoryRouter>,
  );
}

function rowFor(slug: string) {
  return screen.getByText(slug).closest('li') as HTMLElement;
}

describe('AuditCoveragePanel', () => {
  beforeEach(() => {
    grantPermissions(['audit.view']);
    server.use(
      http.get('*/api/audit/coverage', () => HttpResponse.json({ success: true, data: coverage })),
      http.get('*/api/audit/plugins/summary', () => HttpResponse.json({ success: true, data: plugins })),
    );
  });

  it('marks a plugin that is no longer installed', async () => {
    renderPanel();
    expect(await screen.findByText('gone-plugin')).toBeInTheDocument();
    expect(within(rowFor('gone-plugin')).getByText('Uninstalled')).toBeInTheDocument();
  });

  it('leaves an installed plugin unmarked', async () => {
    renderPanel();
    expect(await screen.findByText('sample-tool')).toBeInTheDocument();
    expect(within(rowFor('sample-tool')).queryByText('Uninstalled')).toBeNull();
  });

  it('keeps the audit history of an uninstalled plugin, with its last-event date', async () => {
    renderPanel();
    await screen.findByText('gone-plugin');
    const row = within(rowFor('gone-plugin'));
    expect(row.getByText('2 events')).toBeInTheDocument();
    expect(row.getByText(/Last event 24\/08\/2026/)).toBeInTheDocument();
  });

  it('turns the coverage badge on unmapped required routes, not on the percentage', async () => {
    renderPanel();
    expect(await screen.findByText('90% covered')).toBeInTheDocument();
    expect(screen.getByText('Routes requiring audit')).toBeInTheDocument();
    expect(screen.getByText('Unmapped')).toBeInTheDocument();
  });

  describe('export', () => {
    const exports = vi.fn();
    const saved = vi.fn();

    beforeEach(() => {
      exports.mockReset();
      saved.mockReset();
      URL.createObjectURL = vi.fn(() => 'blob:coverage');
      URL.revokeObjectURL = vi.fn();
      vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
        saved(this.download);
      });
      server.use(
        http.get('*/api/audit/coverage/export', () => {
          exports();
          return HttpResponse.json({ success: true, data: coverage });
        }),
      );
    });

    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('disables Export and names audit.export when it is missing', async () => {
      grantPermissions(['audit.view']);
      const user = userEvent.setup();
      renderPanel();
      const button = await screen.findByRole('button', { name: 'Export' });
      expect(button).toBeDisabled();
      await user.hover(button.parentElement as HTMLElement);
      expect(await screen.findAllByText('Requires the audit.export permission')).not.toHaveLength(0);
      expect(exports).not.toHaveBeenCalled();
    });

    it('requests the export once and saves the file', async () => {
      grantPermissions(['audit.view', 'audit.export']);
      const user = userEvent.setup();
      renderPanel();
      await user.click(await screen.findByRole('button', { name: 'Export' }));
      await waitFor(() => expect(saved).toHaveBeenCalledWith('mcharbor-audit-coverage.json'));
      expect(exports).toHaveBeenCalledTimes(1);
    });
  });
});
