// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MetricsScrapeCard } from './MetricsScrapeCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const copyToClipboard = vi.hoisted(() => vi.fn());
vi.mock('@resources/utils/clipboard', () => ({ copyToClipboard }));

describe('MetricsScrapeCard', () => {
  const scrapes = vi.fn();

  beforeEach(() => {
    scrapes.mockReset();
    copyToClipboard.mockReset();
    copyToClipboard.mockResolvedValue({ ok: true, reason: 'clipboard-api' });
    server.use(
      http.get('*/api/metrics', () => {
        scrapes();
        return new HttpResponse('# HELP a a\n# TYPE a gauge\na 1\n\n', {
          headers: { 'Content-Type': 'text/plain' },
        });
      }),
    );
  });

  it('shows the scrape endpoint and copies it', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<MetricsScrapeCard />);
    const endpoint = `${window.location.origin}/api/metrics`;
    expect(screen.getByText(endpoint)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Copy scrape endpoint' }));
    expect(copyToClipboard).toHaveBeenCalledWith(endpoint);
  });

  it('disables the test and names metrics.scrape when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderWithProviders(<MetricsScrapeCard />);
    const button = screen.getByRole('button', { name: 'Test scrape' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the metrics.scrape permission')).not.toHaveLength(0);
    expect(scrapes).not.toHaveBeenCalled();
  });

  it('makes one request and reports the number of non-empty lines', async () => {
    grantPermissions(['metrics.scrape']);
    const user = userEvent.setup();
    renderWithProviders(<MetricsScrapeCard />);
    await user.click(screen.getByRole('button', { name: 'Test scrape' }));
    expect(await screen.findByText('Scrape succeeded: 3 lines of metrics.')).toBeInTheDocument();
    expect(scrapes).toHaveBeenCalledTimes(1);
  });

  it('shows the server message when the scrape is refused', async () => {
    grantPermissions(['metrics.scrape']);
    const message = 'this API key may not scrape metrics from this instance';
    server.use(
      http.get('*/api/metrics', () =>
        HttpResponse.json({ success: false, error: message, code: 'err.forbidden' }, { status: 403 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<MetricsScrapeCard />);
    await user.click(screen.getByRole('button', { name: 'Test scrape' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
  });
});
