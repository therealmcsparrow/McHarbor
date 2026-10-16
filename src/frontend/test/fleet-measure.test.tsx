// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { IconPlayerStop } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { TablePage } from '@resources/layout/TablePage';
import { renderWithProviders } from './render';
import { server } from './server';

type Row = { id: string; name: string; image: string; state: string; cpu: number; mem: number; error: string };

function fleet(n: number, tick: number): Row[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `c${i}`,
    name: `stack-${Math.floor(i / 8)}-svc-${i % 8}-1`,
    image: `registry.example.com/team/svc-${i % 8}:1.${i % 5}.0`,
    state: i % 17 === 0 ? 'exited' : 'running',
    cpu: (i * 7 + tick) % 100,
    mem: (i * 13 + tick) % 100,
    error: i % 17 === 0 ? 'exit code 137' : '',
  }));
}

const columns: ColumnDef<Row, unknown>[] = [
  { id: 'name', accessorKey: 'name', header: 'Name' },
  { id: 'image', accessorKey: 'image', header: 'Image' },
  {
    id: 'state',
    accessorKey: 'state',
    header: 'State',
    cell: ({ row }) => (
      <div className="flex items-center gap-2">
        <Badge variant={row.original.state === 'running' ? 'success' : 'secondary'}>{row.original.state}</Badge>
        <StatusError error={row.original.error} />
      </div>
    ),
  },
  { id: 'cpu', accessorKey: 'cpu', header: 'CPU' },
  { id: 'mem', accessorKey: 'mem', header: 'Memory' },
  {
    id: 'actions',
    header: 'Actions',
    cell: ({ row }) => (
      <Button variant="ghost" size="icon-sm" aria-label={`stop ${row.original.name}`}>
        <IconPlayerStop className="size-4" />
      </Button>
    ),
  },
];

function Page({ rows, pageSize }: { rows: Row[]; pageSize: readonly number[] }) {
  return (
    <TablePage
      id="fleet-measure"
      title="Fleet"
      columns={columns}
      data={rows}
      selectable
      getRowId={(row) => row.id}
      pagination={{ mode: 'client', pageSizeOptions: pageSize }}
    />
  );
}

const measure = process.env.MCHARBOR_FLEET_MEASURE === '1';

describe.runIf(measure)('fleet render measurement', () => {
  function serveLayout(pageSize: number) {
    const layout = {
      v: 1,
      columns: columns.map((column) => ({ id: column.id, visible: true, size: 'auto' })),
      density: 'default',
      pageSize,
      autoRefreshMs: 0,
    };
    server.use(
      http.get('*/api/table-views/fleet-measure', () =>
        HttpResponse.json({ success: true, data: { layout: { id: 'l', payload: layout }, filters: [] } }),
      ),
    );
  }

  beforeEach(() => serveLayout(25));

  it.each([25, 100, 300])('renders and re-renders %i visible rows', async (visible) => {
    const total = 300;
    serveLayout(visible);
    const view = renderWithProviders(<Page key="warm" rows={fleet(total, 0)} pageSize={[visible]} />);
    await waitFor(() => expect(view.container.querySelector('tbody tr[data-selected], tbody tr')).not.toBeNull());
    await act(async () => {});

    const t0 = performance.now();
    view.rerender(<Page key="measured" rows={fleet(total, 0)} pageSize={[visible]} />);
    await act(async () => {});
    const ticks = 5;
    const t1 = performance.now();
    for (let tick = 1; tick <= ticks; tick++) {
      view.rerender(<Page rows={fleet(total, tick)} pageSize={[visible]} />);
      await act(async () => {});
    }
    const perTick = (performance.now() - t1) / ticks;
    const domRows = view.container.querySelectorAll('tbody tr').length;
    console.info(
      `[fleet] ${visible}-row page of ${total}: ${domRows} <tr> in DOM, first commit ${Math.round(t1 - t0)} ms, ${Math.round(perTick)} ms per stats tick (jsdom)`,
    );
    expect(domRows).toBeGreaterThan(0);
    view.unmount();
  });
});
