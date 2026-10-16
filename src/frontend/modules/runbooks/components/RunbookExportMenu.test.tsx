// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { givePluginCapabilities } from '../../../test/plugin-capabilities';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { RunbookExportMenu } from './RunbookExportMenu';

const reporting = {
  plugin: 'report-plugin',
  kind: 'document-export',
  formats: ['pdf', 'docx'],
  permission: 'reporting.generate',
  generatePath: '/api/reporting/reports',
  exportPath: '/api/reporting/reports/{id}/export',
};

const saved: string[] = [];
vi.mock('@resources/utils/download', async () => {
  const real = await vi.importActual<typeof import('@resources/utils/download')>('@resources/utils/download');
  return {
    ...real,
    downloadFile: async (path: string) => {
      saved.push(path);
    },
  };
});

const generated = vi.fn();

describe('RunbookExportMenu', () => {
  beforeEach(() => {
    saved.length = 0;
    generated.mockReset();
    givePluginCapabilities([reporting]);
    grantPermissions(['runbooks.view', 'reporting.generate']);
    server.use(
      http.post('*/api/reporting/reports', async ({ request }) => {
        generated(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'rep-9' } });
      }),
    );
  });

  it('generates the report scoped to this runbook, then downloads it as a PDF', async () => {
    const user = userEvent.setup();
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);

    await user.click(screen.getByRole('button', { name: 'Export' }));
    await user.click(await screen.findByText('Export as PDF'));

    await waitFor(() => expect(generated).toHaveBeenCalledTimes(1));
    expect(generated).toHaveBeenCalledWith({
      type: 'runbooks',
      subject_kind: 'runbook',
      subject_id: 'rb-1',
      subject_name: 'Restart loop',
    });
    await waitFor(() => expect(saved).toEqual(['/api/reporting/reports/rep-9/export?format=pdf']));
  });

  it('calls the routes the provider declares, whatever plugin it is', async () => {
    givePluginCapabilities([
      {
        ...reporting,
        plugin: 'acme-docs',
        generatePath: '/api/acme-docs/render',
        exportPath: '/api/acme-docs/{id}/file',
      },
    ]);
    const rendered = vi.fn();
    server.use(
      http.post('*/api/acme-docs/render', async ({ request }) => {
        rendered(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'doc 3' } });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);

    await user.click(screen.getByRole('button', { name: 'Export' }));
    await user.click(await screen.findByText('Export as Word document'));

    await waitFor(() => expect(saved).toEqual(['/api/acme-docs/doc%203/file?format=docx']));
    expect(rendered).toHaveBeenCalledTimes(1);
    expect(generated).not.toHaveBeenCalled();
  });

  it('offers JSON alone when the provider names no routes', async () => {
    givePluginCapabilities([{ ...reporting, generatePath: undefined, exportPath: undefined }]);
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);
    expect(screen.getByRole('button', { name: 'Export as JSON' })).toBeInTheDocument();
  });

  it('offers JSON alone when the reporting plugin is not loaded', async () => {
    givePluginCapabilities([reporting], ['report-plugin']);
    const user = userEvent.setup();
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);

    const button = screen.getByRole('button', { name: 'Export as JSON' });
    await user.click(button);

    await waitFor(() => expect(saved).toEqual(['/api/runbooks/rb-1/export']));
    expect(generated).not.toHaveBeenCalled();
    expect(screen.queryByText('Export as PDF')).toBeNull();
  });

  it('offers JSON alone when no plugin declares document export', async () => {
    givePluginCapabilities([]);
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);
    expect(screen.getByRole('button', { name: 'Export as JSON' })).toBeInTheDocument();
  });

  it('offers JSON alone without reporting.generate', async () => {
    grantPermissions(['runbooks.view']);
    renderWithProviders(<RunbookExportMenu id="rb-1" name="Restart loop" />);
    expect(screen.getByRole('button', { name: 'Export as JSON' })).toBeInTheDocument();
  });
});
