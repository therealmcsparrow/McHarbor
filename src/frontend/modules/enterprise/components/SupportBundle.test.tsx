// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { Bundle } from '@resources/hooks/useSupport';
import { SupportBundleForm } from './SupportBundleForm';
import { SupportBundleHistory } from './SupportBundleHistory';

const ready: Bundle = {
  id: 'abcdefghij0123456789',
  status: 'ready',
  categories: ['system', 'logs'],
  createdAt: '2026-09-26T20:00:00Z',
  createdBy: 'admin',
  sizeBytes: 2048,
  progress: 100,
};

describe('Support bundle generation', () => {
  const creates = vi.fn();
  beforeEach(() => {
    creates.mockReset();
    grantPermissions(['support.view', 'support.manage']);
  });

  it('sends the chosen categories once on a double click', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/support/bundles', async ({ request }) => {
        creates(await request.json());
        await gate;
        return HttpResponse.json({ success: true, data: { ...ready, status: 'running' } });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<SupportBundleForm busy={false} />);
    await user.click(screen.getByRole('checkbox', { name: /Recent server logs/ }));
    await user.dblClick(screen.getByRole('button', { name: 'Generate bundle' }));
    release();
    await waitFor(() => expect(creates).toHaveBeenCalledTimes(1));
    expect(creates).toHaveBeenCalledWith({
      categories: ['system', 'plugins', 'cluster', 'database', 'environments', 'audit', 'diagnostics'],
    });
  });

  it('renders generate disabled, naming support.manage, for a view-only user', async () => {
    grantPermissions(['support.view']);
    const user = userEvent.setup();
    renderWithProviders(<SupportBundleForm busy={false} />);
    const button = screen.getByRole('button', { name: 'Generate bundle' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(creates).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the support.manage permission')).not.toHaveLength(0);
  });

  it('shows the server message when generation is refused', async () => {
    server.use(
      http.post('*/api/support/bundles', () =>
        HttpResponse.json(
          { success: false, error: 'A support bundle is already being generated on this node.' },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<SupportBundleForm busy={false} />);
    await user.click(screen.getByRole('button', { name: 'Generate bundle' }));
    expect(await screen.findByText(/A support bundle is already being generated on this node\./)).toBeTruthy();
  });

  it('shows exactly what the server says the bundle will contain before anything is generated', async () => {
    server.use(
      http.get('*/api/support/bundles/preview', ({ request }) => {
        expect(new URL(request.url).searchParams.get('categories')).toBe(
          'system,logs,plugins,cluster,database,environments,audit,diagnostics',
        );
        return HttpResponse.json({
          success: true,
          data: {
            categories: ['system'],
            totalBytes: 42,
            files: [{ name: 'system.json', sizeBytes: 42, content: '{"MCHARBOR_SECRET": "[redacted]"}' }],
          },
        });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<SupportBundleForm busy={false} />);
    await user.click(screen.getByRole('button', { name: 'Preview redaction' }));
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText('{"MCHARBOR_SECRET": "[redacted]"}')).toBeTruthy();
    expect(creates).not.toHaveBeenCalled();
  });

  it('disables generating while another bundle is running', () => {
    renderWithProviders(<SupportBundleForm busy />);
    expect(screen.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
  });
});

describe('Support bundle history', () => {
  it('renders download disabled, naming support.manage, for a view-only user', async () => {
    grantPermissions(['support.view']);
    const user = userEvent.setup();
    renderWithProviders(<SupportBundleHistory list={{ items: [ready], keep: 10 }} />);
    const download = screen.getByRole('button', { name: 'Download' });
    expect(download).toBeDisabled();
    await user.hover(download.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the support.manage permission')).not.toHaveLength(0);
  });

  it('puts a failed bundle behind the status error icon', () => {
    grantPermissions(['support.view', 'support.manage']);
    renderWithProviders(
      <SupportBundleHistory list={{ items: [{ ...ready, status: 'failed', error: 'generation_failed' }], keep: 10 }} />,
    );
    expect(screen.getByText('Failed')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Download' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'View the error' })).toBeTruthy();
  });
});
