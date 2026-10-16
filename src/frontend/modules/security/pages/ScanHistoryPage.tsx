// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect, useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBug } from '@tabler/icons-react';
import { Select } from '@resources/components/ui/Select';
import { TablePage } from '@resources/layout/TablePage';
import { useDeleteScan, useAllScans } from '@resources/hooks/useScans';
import { useScanHistoryColumns } from '../components/ScanHistoryColumns';

const PER_PAGE = 25;
const SEARCH_DEBOUNCE_MS = 300;

const STATUS_OPTIONS = [
  { value: '', key: 'scanHistory.status.all' },
  { value: 'pending', key: 'scanHistory.status.pending' },
  { value: 'running', key: 'scanHistory.status.running' },
  { value: 'completed', key: 'scanHistory.status.completed' },
  { value: 'failed', key: 'scanHistory.status.failed' },
] as const;

export default function ScanHistoryPage() {
  const { t } = useTranslation('security');
  const [page, setPage] = useState(1);
  const [statusValue, setStatus] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');

  useEffect(() => {
    const timeout = setTimeout(() => {
      setSearch(searchInput);
      setPage(1);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timeout);
  }, [searchInput]);

  const { data, isLoading, isFetching, refetch } = useAllScans({
    page,
    perPage: PER_PAGE,
    status: statusValue,
    search,
  });
  const deleteScan = useDeleteScan();

  const items = useMemo(() => data?.items ?? [], [data]);
  const total = data?.total ?? 0;
  const perPage = data?.per_page ?? PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const from = total === 0 ? 0 : (page - 1) * perPage + 1;
  const to = items.length === 0 ? 0 : from + items.length - 1;

  const handleDelete = useCallback((scan: { id: string }) => deleteScan.mutate(scan.id), [deleteScan]);

  const columns = useScanHistoryColumns({
    deleting: deleteScan.isPending,
    onDelete: handleDelete,
  });

  return (
    <TablePage
      id="scan-history"
      title={t('scanHistory.title')}
      description={t('scanHistory.description')}
      search={{
        value: searchInput,
        onChange: setSearchInput,
        mode: 'contains',
        onModeChange: () => undefined,
        placeholder: t('scanHistory.searchPlaceholder'),
      }}
      savedFilters={{
        state: { status: statusValue, search },
        onRestore: (s) => {
          const nextStatus = typeof s.status === 'string' ? s.status : '';
          const nextSearch = typeof s.search === 'string' ? s.search : '';
          setStatus(nextStatus);
          setSearchInput(nextSearch);
          setSearch(nextSearch);
          setPage(1);
        },
        pristine: statusValue === '' && search === '',
        defaults: { status: '', search: '' },
      }}
      filters={
        <Select
          value={statusValue}
          onChange={(value) => {
            setStatus(value);
            setPage(1);
          }}
          options={STATUS_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.key) }))}
          ariaLabel={t('scanHistory.status.all')}
          className="w-48"
        />
      }
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: { filename: 'mcharbor-scan-history' },
        chips: {
          items: statusValue
            ? [
                {
                  id: 'status',
                  label: t(STATUS_OPTIONS.find((opt) => opt.value === statusValue)?.key ?? statusValue),
                  onRemove: () => {
                    setStatus('');
                    setPage(1);
                  },
                },
              ]
            : [],
          onClear: () => {
            setStatus('');
            setSearchInput('');
            setSearch('');
            setPage(1);
          },
        },
      }}
      columns={columns}
      data={items}
      loading={isLoading && items.length === 0}
      empty={{ icon: <IconBug className="size-8" />, title: t('scanHistory.empty') }}
      getRowId={(scan) => scan.id}
      pagination={{
        mode: 'server',
        page,
        totalPages,
        pageSize: perPage,
        total,
        from,
        to,
        onPageChange: setPage,
        // Page size is fixed by the API call this page makes.
        onPageSizeChange: () => undefined,
        pageSizeOptions: [PER_PAGE],
      }}
    />
  );
}
