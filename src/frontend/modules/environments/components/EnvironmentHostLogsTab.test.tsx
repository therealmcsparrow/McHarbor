// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { EnvironmentHostLogsTab } from './EnvironmentHostLogsTab';

const LINES = ['06:40 first line', '06:41 second line', '06:42 third line'];

function shownLines(): string[] {
  return LINES.map((line) => ({ line, at: document.body.textContent?.indexOf(line) ?? -1 }))
    .filter((entry) => entry.at >= 0)
    .sort((a, b) => a.at - b.at)
    .map((entry) => entry.line);
}

describe('EnvironmentHostLogsTab', () => {
  beforeEach(() => {
    server.use(
      http.get('*/api/system/os-logs', () =>
        HttpResponse.json({ success: true, data: { lines: LINES, source: 'system' } }),
      ),
    );
  });

  it('shows the newest line first, and the oldest first after toggling the order', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentHostLogsTab envId="env1" />);

    await screen.findByText('06:42 third line');
    expect(shownLines()).toEqual(['06:42 third line', '06:41 second line', '06:40 first line']);

    await user.click(screen.getByRole('button', { name: 'Newest first' }));
    expect(await screen.findByRole('button', { name: 'Oldest first' })).toBeInTheDocument();
    expect(shownLines()).toEqual(LINES);
  });

  it('starts with live refresh off and switches it on', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentHostLogsTab envId="env1" />);

    const live = await screen.findByRole('button', { name: 'Live' });
    expect(live).toHaveAttribute('aria-pressed', 'false');
    await user.click(live);
    expect(screen.getByRole('button', { name: 'Live' })).toHaveAttribute('aria-pressed', 'true');
  });
});

const JOURNAL = [
  'Oct 08 10:00:01 host sshd[1]: accepted password for admin',
  'Oct 08 10:00:02 host kernel: error: disk sda failed',
  'Oct 08 10:00:03 host cron[2]: job finished',
];

function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.readAsText(blob);
  });
}

describe('EnvironmentHostLogsTab filtering and export', () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:host-log');

  beforeEach(() => {
    server.use(
      http.get('*/api/system/os-logs', () =>
        HttpResponse.json({ success: true, data: { lines: JOURNAL, source: 'system' } }),
      ),
    );
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
    // The dropdown popover measures itself; jsdom has no ResizeObserver.
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    createObjectURL.mockClear();
    vi.unstubAllGlobals();
  });

  it('narrows the lines by the text filter', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentHostLogsTab envId="env1" />);
    await screen.findByText(/job finished/);

    await user.type(screen.getByRole('textbox', { name: 'Filter log records' }), 'sshd');

    expect(screen.getByText(/accepted password/)).toBeInTheDocument();
    expect(screen.queryByText(/job finished/)).not.toBeInTheDocument();
  });

  it('narrows the lines by severity level', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentHostLogsTab envId="env1" />);
    await screen.findByText(/job finished/);

    await user.click(screen.getByRole('button', { name: 'Log level' }));
    await user.click(await screen.findByRole('button', { name: 'Error' }));

    expect(screen.getByText(/disk sda failed/)).toBeInTheDocument();
    expect(screen.queryByText(/job finished/)).not.toBeInTheDocument();
  });

  it('exports exactly the lines the filters left visible', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnvironmentHostLogsTab envId="env1" />);
    await screen.findByText(/job finished/);
    await user.type(screen.getByRole('textbox', { name: 'Filter log records' }), 'sshd');

    await user.click(screen.getByRole('button', { name: 'Export' }));

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(await readBlob(createObjectURL.mock.calls[0]![0])).toBe(JOURNAL[0]);
  });
});
