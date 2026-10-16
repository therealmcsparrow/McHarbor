// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { IconPlus, IconServerOff, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { SearchMode } from '@resources/utils/search-filter';
import { TablePage } from './TablePage';
import type { ActiveFilterChip, TableColumnMeta } from './table-page/types';

type Row = {
  id: string;
  name: string;
  status: 'running' | 'stopped' | 'restarting';
  image: string;
  created: string;
};

const statusVariant: Record<Row['status'], 'success' | 'destructive' | 'warning'> = {
  running: 'success',
  stopped: 'destructive',
  restarting: 'warning',
};

const STATUSES: Row['status'][] = ['running', 'stopped', 'restarting'];

const rows: Row[] = Array.from({ length: 60 }, (_, index) => ({
  id: `row-${index}`,
  name: `service-${String(index + 1).padStart(2, '0')}`,
  status: STATUSES[index % STATUSES.length] ?? 'running',
  image: `ghcr.io/mcsparrow/sample:${1 + (index % 5)}.0`,
  created: `2026-0${1 + (index % 9)}-14`,
}));

const actionsMeta: TableColumnMeta = { width: 'xs', locked: true, noExport: true };

const columns: ColumnDef<Row, unknown>[] = [
  { accessorKey: 'name', header: 'Name', meta: { width: 'lg' } satisfies TableColumnMeta },
  {
    accessorKey: 'status',
    header: 'Status',
    meta: { width: 'sm' } satisfies TableColumnMeta,
    cell: ({ row }) => <Badge variant={statusVariant[row.original.status]}>{row.original.status}</Badge>,
  },
  { accessorKey: 'image', header: 'Image' },
  { accessorKey: 'created', header: 'Created', meta: { width: 'md' } satisfies TableColumnMeta },
  {
    id: 'actions',
    header: 'Actions',
    meta: actionsMeta,
    cell: () => (
      <Button variant="outline" size="icon-sm" aria-label="Remove">
        <IconTrash className="size-3.5" />
      </Button>
    ),
  },
];

export default {
  title: 'Layout / TablePage',
};

function useDemoSearch() {
  const [value, setValue] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  return {
    value,
    onChange: setValue,
    mode,
    onModeChange: setMode,
    placeholder: 'Search services...',
  };
}

/** The full band: search, saved filters, page filters, and all four toolbar groups. */
export const Default = () => {
  const search = useDemoSearch();
  const [status, setStatus] = useState('');
  const chips: ActiveFilterChip[] = status
    ? [{ id: 'status', label: `status: ${status}`, onRemove: () => setStatus('') }]
    : [];

  return (
    <div className="h-[80vh] p-6">
      <TablePage
        id="story-default"
        title="Services"
        description="Every control this layout offers, on one page."
        actions={
          <Button>
            <IconPlus className="size-4" />
            New service
          </Button>
        }
        search={search}
        savedFilters={{ state: { query: search.value, status }, onRestore: () => undefined }}
        filters={
          <Select
            value={status}
            onChange={setStatus}
            options={[
              { value: '', label: 'All statuses' },
              { value: 'running', label: 'Running' },
              { value: 'stopped', label: 'Stopped' },
            ]}
            className="w-48"
          />
        }
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => undefined, isFetching: false },
          export: { filename: 'services' },
          chips: { items: chips, onClear: () => setStatus('') },
        }}
        columns={columns}
        data={rows}
        getRowId={(row) => row.id}
      />
    </div>
  );
};

/** Server-paged: the pager numbers come from query state, not from TanStack. */
export const ServerPaged = () => (
  <div className="h-[80vh] p-6">
    <TablePage
      id="story-server"
      title="Audit log"
      description="Paged by the API. Client-side export covers the current page only."
      toolbar={{ columns: true, density: true, export: { filename: 'audit' } }}
      columns={columns}
      data={rows.slice(0, 25)}
      getRowId={(row) => row.id}
      pagination={{
        mode: 'server',
        page: 2,
        totalPages: 8,
        pageSize: 25,
        total: 193,
        from: 26,
        to: 50,
        onPageChange: () => undefined,
        onPageSizeChange: () => undefined,
      }}
    />
  </div>
);

/** Selection swaps row 1 into the bulk bar in place — no fourth band appears. */
export const WithSelection = () => {
  const search = useDemoSearch();
  return (
    <div className="h-[80vh] p-6">
      <TablePage
        id="story-selection"
        title="Services"
        description="Select a row to see the filter row swap into the bulk bar."
        search={search}
        selectable
        batchActions={[
          { label: 'Stop', icon: IconServerOff, onClick: () => undefined },
          { label: 'Remove', icon: IconTrash, variant: 'destructive', confirm: true, onClick: () => undefined },
        ]}
        toolbar={{ columns: true, density: true }}
        columns={columns}
        data={rows}
        getRowId={(row) => row.id}
      />
    </div>
  );
};

export const Loading = () => (
  <div className="h-[80vh] p-6">
    <TablePage id="story-loading" title="Services" loading columns={columns} data={[]} />
  </div>
);

export const Empty = () => (
  <div className="h-[80vh] p-6">
    <TablePage
      id="story-empty"
      title="Services"
      columns={columns}
      data={[]}
      empty={{ title: 'No services yet', description: 'Create one to get started.' }}
    />
  </div>
);

export const ErrorState = () => (
  <div className="h-[80vh] p-6">
    <TablePage
      id="story-error"
      title="Services"
      columns={columns}
      data={[]}
      error={new Error('Docker daemon unreachable')}
      onRetry={() => undefined}
    />
  </div>
);

/** No search, no saved filters, no toolbar — the band must not render at all. */
export const NoToolbar = () => (
  <div className="h-[80vh] p-6">
    <TablePage
      id="story-bare"
      title="Services"
      columns={columns}
      data={rows.slice(0, 10)}
      getRowId={(row) => row.id}
      pagination={{ mode: 'none' }}
    />
  </div>
);
