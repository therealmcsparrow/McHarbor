// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { screen, waitFor, act, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AppsTab from './AppStorePage';
import type { AppTemplate } from '../types';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const TOTAL = 130;

function app(i: number): AppTemplate {
  return {
    id: `id-${i}`,
    slug: `app-${i}`,
    name: `App ${i}`,
    description: '',
    category: 'Tools',
    image: `example/app-${i}:latest`,
    logo: '',
    website: '',
    docsUrl: '',
    ports: [],
    volumes: [],
    envVars: [],
    source: 'local',
    version: '1.0.0',
    updatedAt: '2026-09-29T00:00:00Z',
    installed: false,
    installations: [],
  };
}

const requestedPages: number[] = [];

function useCatalog(total = TOTAL) {
  server.use(
    http.get('*/api/app-store', ({ request }) => {
      const url = new URL(request.url);
      const page = Number(url.searchParams.get('page'));
      const perPage = Number(url.searchParams.get('per_page'));
      requestedPages.push(page);
      const start = (page - 1) * perPage;
      const items = Array.from({ length: Math.max(0, Math.min(perPage, total - start)) }, (_, k) => app(start + k));
      return HttpResponse.json({
        success: true,
        data: { items, total, page, per_page: perPage, total_pages: Math.ceil(total / perPage) },
      });
    }),
    http.get('*/api/app-store/categories', () =>
      HttpResponse.json({ success: true, data: [{ category: 'Tools', count: total }] }),
    ),
    http.get('*/api/app-store/sync/status', () => HttpResponse.json({ success: true, data: null })),
  );
}

type ObserverCallback = (entries: Array<{ isIntersecting: boolean }>) => void;
let observers: ObserverCallback[] = [];

beforeEach(() => {
  requestedPages.length = 0;
  observers = [];
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(cb: ObserverCallback) {
        observers.push(cb);
      }
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ByRole walks the accessibility tree of every card (~300 ms a query at 130 cards),
// which pushed this suite past the test timeout under full-run load.
const cards = () => screen.queryAllByText(/^App \d+$/, { selector: 'h3' });
const footer = () => within(screen.getByText(/^Showing \d+ of \d+ apps$/).parentElement as HTMLElement);

describe('AppStorePage catalog paging', () => {
  it('shows the first page and says how many of the total are loaded', async () => {
    useCatalog();
    renderWithProviders(<AppsTab />);
    await screen.findByText('App 0');
    expect(cards()).toHaveLength(60);
    expect(screen.getByText('Showing 60 of 130 apps')).toBeInTheDocument();
  });

  it('loads every remaining page through Load more until the catalog is complete', async () => {
    useCatalog();
    const user = userEvent.setup();
    renderWithProviders(<AppsTab />);
    await screen.findByText('App 0');

    await user.click(footer().getByRole('button', { name: 'Load more' }));
    await screen.findByText('App 119');
    await user.click(footer().getByRole('button', { name: 'Load more' }));
    await screen.findByText('App 129');

    expect(cards()).toHaveLength(TOTAL);
    expect(screen.getByText('Showing 130 of 130 apps')).toBeInTheDocument();
    expect(footer().queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();
  });

  it('fetches the next page when the end of the grid scrolls into view', async () => {
    useCatalog();
    renderWithProviders(<AppsTab />);
    await screen.findByText('App 0');
    await waitFor(() => expect(observers.length).toBeGreaterThan(0));

    act(() => observers.at(-1)?.([{ isIntersecting: true }]));

    await screen.findByText('App 119');
    expect(requestedPages).toContain(2);
  });

  it('offers no Load more when the catalog fits on one page', async () => {
    useCatalog(12);
    renderWithProviders(<AppsTab />);
    await screen.findByText('App 11');
    expect(cards()).toHaveLength(12);
    expect(footer().queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();
  });
});
