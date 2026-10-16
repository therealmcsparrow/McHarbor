// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { AboutTab } from './AboutTab';
import { WhatsNewOverlay } from '@resources/components/WhatsNewOverlay';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

vi.mock('./SecurityPostureCard', () => ({ SecurityPostureCard: () => null }));
vi.mock('./FactoryResetCard', () => ({ FactoryResetCard: () => null }));

describe('AboutTab', () => {
  it('shows the server message when the about request fails, and retries on request', async () => {
    const user = userEvent.setup();
    const loads = vi.fn();
    server.use(
      http.get('*/api/about', () => {
        loads();
        return HttpResponse.json({ success: false, error: 'Build metadata is unavailable.' }, { status: 500 });
      }),
    );
    renderWithProviders(<AboutTab />);

    expect(await screen.findByText('Build metadata is unavailable.')).toBeInTheDocument();
    expect(loads).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(loads).toHaveBeenCalledTimes(2));
  });

  it('renders no error state while the about request succeeds', async () => {
    server.use(
      http.get('*/api/about', () =>
        HttpResponse.json({
          success: true,
          data: {
            version: '2026.09.01',
            build: '202609181858',
            goVersion: 'go1.27.1',
            platform: 'linux/amd64',
            dependencies: [],
          },
        }),
      ),
    );
    renderWithProviders(<AboutTab />);

    expect(await screen.findByText('202609181858')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
  });

  // Placed between Website and Repository, and works for a viewer who is not an
  // admin — only the automatic upgrade popup is admin-only.
  it("opens the release highlights from the What's new button", async () => {
    const user = userEvent.setup();
    server.use(
      http.get('*/api/about', () =>
        HttpResponse.json({
          success: true,
          data: {
            version: '2026.10.01',
            build: '202610021055',
            goVersion: 'go1.27.1',
            platform: 'linux/amd64',
            dependencies: [],
          },
        }),
      ),
    );
    renderWithProviders(
      <>
        <AboutTab />
        <WhatsNewOverlay />
      </>,
    );

    const button = await screen.findByRole('button', { name: "What's new" });
    await waitFor(() => expect(button).toBeEnabled());
    const links = screen.getAllByRole('link').map((link) => link.textContent);
    expect(links).toEqual(['Website', 'GitHub repo']);
    const website = screen.getByRole('link', { name: 'Website' });
    const repository = screen.getByRole('link', { name: 'GitHub repo' });
    expect(website.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(button.compareDocumentPosition(repository) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    await user.click(button);
    expect(await screen.findByRole('dialog', { name: "What's new in 2026.10.01" })).toBeInTheDocument();
    expect(screen.getByText('Network and disk charts show live throughput')).toBeInTheDocument();
  });
});
