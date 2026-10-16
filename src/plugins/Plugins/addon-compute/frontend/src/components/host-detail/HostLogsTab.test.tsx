// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { HostLogsTab } from './HostLogsTab';

const get = vi.hoisted(() => vi.fn());
vi.mock('@core/api/client', () => ({ api: { get }, envelopeStatus: () => undefined }));
vi.mock('@core/auth/usePermission', () => ({ usePermission: () => true }));

function event(id: string, severity: string, action: string) {
  return {
    id,
    subjectType: 'vm',
    subjectId: id,
    subjectName: id,
    eventType: 'lifecycle',
    action,
    severity,
    source: 'test',
    timestamp: '2026-10-08T10:00:00Z',
  };
}

function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <HostLogsTab envId="env1" />
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

describe('HostLogsTab', () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:host-log');

  beforeEach(() => {
    get.mockReset();
    get.mockResolvedValue({
      success: true,
      data: { items: [event('alpha', 'info', 'start'), event('bravo', 'error', 'crash')] },
    });
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
  });

  afterEach(() => createObjectURL.mockClear());

  it('offers the shared log options, not only a substring filter', async () => {
    mount();
    await screen.findByText(/alpha/);

    for (const name of ['Regex', 'Newest first', 'Live', 'Export']) {
      expect(screen.getByRole('button', { name })).toBeInTheDocument();
    }
    expect(screen.getByRole('button', { name: 'Log level' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Time range' })).toBeInTheDocument();
  });

  it('narrows the lines with the shared text filter', async () => {
    const user = userEvent.setup();
    mount();
    await screen.findByText(/bravo/);

    await user.type(screen.getByRole('textbox', { name: 'Filter log records' }), 'alpha');

    expect(screen.getByText(/alpha/)).toBeInTheDocument();
    expect(screen.queryByText(/bravo/)).not.toBeInTheDocument();
  });

  it('exports the lines that survive the filter', async () => {
    const user = userEvent.setup();
    mount();
    await screen.findByText(/bravo/);
    await user.type(screen.getByRole('textbox', { name: 'Filter log records' }), 'bravo');

    await user.click(screen.getByRole('button', { name: 'Export' }));

    await waitFor(() => expect(createObjectURL).toHaveBeenCalledTimes(1));
    const text = await new Promise<string>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
      reader.readAsText(createObjectURL.mock.calls[0]![0]);
    });
    expect(text).toContain('bravo');
    expect(text).not.toContain('alpha');
  });
});
