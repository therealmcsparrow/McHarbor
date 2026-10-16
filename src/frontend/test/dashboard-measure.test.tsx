// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense } from 'react';
import { MemoryRouter } from 'react-router';
import { act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WIDGET_REGISTRY } from '@/modules/dashboard/widgets/registry';
import type { WidgetTypeId } from '@/modules/dashboard/widgets/catalog';
import { WidgetErrorBoundary } from '@/modules/dashboard/components/WidgetErrorBoundary';
import { EstateSummary } from '@/modules/dashboard/components/EstateSummary';
import { useEnvironmentStore } from '@resources/stores/environment';
import { renderWithProviders } from './render';
import { server } from './server';
import { grantPermissions, resetPermissions } from './permissions';

const DEFAULT_LAYOUT: WidgetTypeId[] = [
  'fleet-health',
  'needs-attention',
  'update-queue',
  'containers',
  'images',
  'volumes',
  'networks',
  'cpu-chart',
  'memory-chart',
  'network-io-chart',
  'disk-io-chart',
  'cpu-cores',
  'total-memory',
  'docker-version',
  'disk-usage',
];

type Hit = { key: string; at: number };

function record(): Hit[] {
  const hits: Hit[] = [];
  server.use(
    http.all('*/api/*', ({ request }) => {
      const url = new URL(request.url);
      url.searchParams.delete('_');
      hits.push({ key: `${request.method} ${url.pathname}${url.search}`, at: Date.now() });
      return HttpResponse.json({ success: true, data: [] });
    }),
  );
  return hits;
}

function Dashboard({ types }: { types: WidgetTypeId[] }) {
  return (
    <MemoryRouter>
      <EstateSummary />
      {types.map((typeId) => {
        const Widget = WIDGET_REGISTRY[typeId]?.component;
        if (!Widget) return null;
        return (
          <WidgetErrorBoundary key={typeId} title={typeId}>
            <Suspense fallback={null}>
              <Widget typeId={typeId} colSpan={4} />
            </Suspense>
          </WidgetErrorBoundary>
        );
      })}
    </MemoryRouter>
  );
}

function summarise(hits: Hit[], from: number, to: number) {
  const window = hits.filter((hit) => hit.at >= from && hit.at < to);
  const byKey = new Map<string, number>();
  for (const hit of window) byKey.set(hit.key, (byKey.get(hit.key) ?? 0) + 1);
  return { total: window.length, distinct: byKey.size, byKey };
}

const measure = process.env.MCHARBOR_FLEET_MEASURE === '1';

describe.runIf(measure)('dashboard request load', () => {
  afterEach(() => {
    vi.useRealTimers();
    resetPermissions();
  });

  it.each([
    ['default layout (15 widgets)', DEFAULT_LAYOUT],
    ['every widget type', Object.keys(WIDGET_REGISTRY) as WidgetTypeId[]],
  ])(
    '%s',
    async (label, types) => {
      const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
      grantPermissions('*');
      useEnvironmentStore.setState({ currentId: 'env-1' });
      await Promise.all(Object.values(import.meta.glob('@widgets/*/*Widget.tsx')).map((load) => load()));
      vi.useFakeTimers({ shouldAdvanceTime: true });
      const hits = record();
      const t0 = Date.now();
      renderWithProviders(<Dashboard types={types} />);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2_000);
      });
      const load = summarise(hits, t0, t0 + 2_000);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_000);
      });
      const minute = summarise(hits, t0 + 2_000, t0 + 62_000);
      const perKey = [...minute.byKey.entries()].sort((a, b) => b[1] - a[1]).map(([k, n]) => `${n}× ${k}`);
      console.info(
        `[dashboard] ${label}: ${types.length} widgets → ${load.total} requests on load (${load.distinct} distinct), ${minute.total} in the next minute (${minute.distinct} distinct)\n  load: ${[...load.byKey.keys()].join(', ')}\n  minute: ${perKey.join(', ')}`,
      );
      console.info(
        `[dashboard] errors: ${errors.mock.calls
          .slice(0, 3)
          .map((c) => String(c[0]).slice(0, 200))
          .join(' | ')}`,
      );
      expect(load.total).toBeGreaterThan(0);
    },
    180_000,
  );
});
