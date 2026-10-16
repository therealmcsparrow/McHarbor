// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { CLIDownloadDialog } from './CLIDownloadDialog';

const archives = [
  {
    os: 'linux',
    arch: 'amd64',
    file: 'mcharbor_2026.09.01_linux_amd64.tar.gz',
    version: '2026.09.01',
    size: 5_242_880,
    sha256: 'a'.repeat(64),
  },
  {
    os: 'windows',
    arch: 'amd64',
    file: 'mcharbor_2026.09.01_windows_amd64.zip',
    version: '2026.09.01',
    size: 5_242_880,
  },
];

function mount() {
  renderWithProviders(<CLIDownloadDialog open onOpenChange={() => undefined} />);
}

describe('CLIDownloadDialog', () => {
  it('offers one download per platform the server has, pointing at the download route', async () => {
    server.use(http.get('*/api/cli/downloads', () => HttpResponse.json({ success: true, data: archives })));
    mount();
    const linux = await screen.findByRole('link', { name: 'Download for Linux amd64' });
    expect(linux).toHaveAttribute('href', '/api/cli/download/linux/amd64');
    expect(screen.getByRole('link', { name: 'Download for Windows amd64' })).toHaveAttribute(
      'href',
      '/api/cli/download/windows/amd64',
    );
  });

  it('copies a sign-in command for this very server', async () => {
    server.use(http.get('*/api/cli/downloads', () => HttpResponse.json({ success: true, data: archives })));
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });
    mount();
    await user.click(await screen.findByRole('button', { name: 'Copy the sign-in command' }));
    expect(writeText).toHaveBeenCalledWith(`mcharbor login ${window.location.origin}`);
    expect(await screen.findByText('Command copied')).toBeInTheDocument();
  });

  it('adds --insecure to the sign-in command when the certificate is self-signed', async () => {
    server.use(http.get('*/api/cli/downloads', () => HttpResponse.json({ success: true, data: archives })));
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });
    mount();
    await user.click(await screen.findByRole('switch', { name: 'Server uses a self-signed certificate' }));
    expect(screen.getByText(`mcharbor login --insecure ${window.location.origin}`)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Copy the sign-in command' }));
    expect(writeText).toHaveBeenCalledWith(`mcharbor login --insecure ${window.location.origin}`);
    await user.click(screen.getByRole('switch', { name: 'Server uses a self-signed certificate' }));
    expect(screen.getByText(`mcharbor login ${window.location.origin}`)).toBeInTheDocument();
  });

  it('says so when the server ships no archives', async () => {
    server.use(http.get('*/api/cli/downloads', () => HttpResponse.json({ success: true, data: [] })));
    mount();
    expect(await screen.findByText('No client archives are available on this server.')).toBeInTheDocument();
  });

  it("keeps the server's own message behind the error icon when listing fails", async () => {
    server.use(
      http.get('*/api/cli/downloads', () =>
        HttpResponse.json({ success: false, error: 'cli-bin is not readable', code: 'err.internal' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    mount();
    expect(await screen.findByText('The client downloads could not be listed.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(/cli-bin is not readable/)).toBeInTheDocument();
  });
});
