// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { i18n, initTestI18n } from '../../../test/i18n';
import widgetEn from '../../../../../widgets/drift-status/i18n/en.json';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import DriftReportPage from '../pages/DriftReportPage';
import DriftStatusWidget, { groupByWatch } from '@widgets/drift-status/DriftStatusWidget';
import type { DriftReportRow } from '../hooks/useDriftReport';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const rows: DriftReportRow[] = [
  {
    desiredStateId: 'a',
    name: 'shop',
    kind: 'stack',
    envId: 'e1',
    class: 'state',
    field: 'service.db',
    expected: 'running',
    actual: 'exited',
    firstSeenAt: '2026-09-25T00:00:00Z',
    checkedAt: '2026-09-26T00:00:00Z',
  },
  {
    desiredStateId: 'b',
    name: 'prod',
    kind: 'environment',
    envId: 'e1',
    class: 'existence',
    field: 'undeclared.debug',
    expected: 'not present',
    actual: 'alpine:3',
    firstSeenAt: '2026-09-20T00:00:00Z',
    checkedAt: '2026-09-26T00:00:00Z',
  },
  {
    desiredStateId: 'a',
    name: 'shop',
    kind: 'stack',
    envId: 'e1',
    class: 'existence',
    field: 'service.sidecar',
    expected: 'not declared',
    actual: 'x',
    firstSeenAt: '2026-09-26T00:00:00Z',
    checkedAt: '2026-09-26T00:00:00Z',
  },
];

function withShell(node: React.ReactElement) {
  if (!document.getElementById('pagehead-slot')) {
    const slot = document.createElement('div');
    slot.id = 'pagehead-slot';
    document.body.appendChild(slot);
  }
  return renderWithProviders(<MemoryRouter>{node}</MemoryRouter>);
}

describe('Drift report and widget', () => {
  beforeAll(async () => {
    await initTestI18n();
    i18n.addResourceBundle('en', 'dashboard', widgetEn, true, true);
  });

  it('lists every drift and sends the class filter to the server', async () => {
    grantPermissions(['reconciler.view']);
    const seen = vi.fn();
    server.use(
      http.get('*/api/table-views/reconciler-report', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
      http.get('*/api/reconciler/report', ({ request }) => {
        const cls = new URL(request.url).searchParams.get('class') ?? '';
        seen(cls);
        return HttpResponse.json({ success: true, data: cls ? rows.filter((r) => r.class === cls) : rows });
      }),
    );
    const user = userEvent.setup();
    withShell(<DriftReportPage />);
    expect(await screen.findByText('undeclared.debug')).toBeInTheDocument();
    expect(screen.getByText('service.sidecar')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Class' }));
    await user.click(await screen.findByRole('button', { name: 'State' }));
    await waitFor(() => expect(seen).toHaveBeenLastCalledWith('state'));
    await waitFor(() => expect(screen.queryByText('undeclared.debug')).not.toBeInTheDocument());
  });

  it('groups the widget by watch, longest wrong first', () => {
    const watches = groupByWatch(rows);
    expect(watches.map((w) => w.id)).toEqual(['b', 'a']);
    expect(watches[1]).toMatchObject({
      count: 2,
      classes: ['state', 'existence'],
      firstSeenAt: '2026-09-25T00:00:00Z',
    });
  });

  it('shows the widget as locked, and asks nothing, without reconciler.view', async () => {
    grantPermissions([]);
    const seen = vi.fn();
    server.use(
      http.get('*/api/reconciler/report', () => {
        seen();
        return HttpResponse.json({ success: true, data: rows });
      }),
    );
    withShell(<DriftStatusWidget colSpan={6} typeId="drift-status" />);
    expect(await screen.findByText('Needs the reconciler.view permission')).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(seen).not.toHaveBeenCalled();
  });
});
