// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArchive } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { Select } from '@resources/components/ui/Select';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import type { ActiveFilterChip } from '@resources/layout/table-page/types';
import { useCatalog, useDeleteReport, useExport, useReports } from '../hooks/useReporting';
import { useLibraryColumns } from './LibraryColumns';
import type { StoredReport } from '../types';
import { useTypeName } from './catalog-labels';

// The library: every report generated so far.
//
// A stored report is a fixed record of what was true when it ran, which
// is what makes it worth keeping — so the row shows the score it had, not
// the score the platform has now.
//
// The page owns no chrome of its own: header, filter band, toolbar, grid
// and pager all come from the host's `TablePage`, the same layout the
// twenty host list pages use. That is what keeps row density, the columns
// menu, export and the page-size control identical to Containers or Audit.

type Props = { onOpen: (id: string) => void };

export function LibraryPage({ onOpen }: Props) {
  const { t } = useTranslation('reporting');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [type, setType] = useState('');
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [pendingDelete, setPendingDelete] = useState<StoredReport | null>(null);

  const catalog = useCatalog();
  const reports = useReports(page, pageSize, type || undefined);
  const remove = useDeleteReport();
  const download = useExport();

  const items = useMemo(() => reports.data?.items ?? [], [reports.data]);
  const total = reports.data?.total ?? 0;
  const totalPages = Math.max(reports.data?.total_pages ?? 1, 1);
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = items.length === 0 ? 0 : from + items.length - 1;

  // Resolves a contributed type's name from its own plugin's bundle;
  // a built-in one still comes from this plugin's `type.<key>.name`.
  const typeLabel = useTypeName();

  const columns = useLibraryColumns({
    onDownload: (report, format) => download.mutate({ id: report.id, format, name: report.title }),
    onDelete: (report) => setPendingDelete(report),
  });

  const chips: ActiveFilterChip[] = type
    ? [
        {
          id: 'type',
          label: typeLabel(type),
          onRemove: () => {
            setType('');
            setPage(1);
          },
        },
      ]
    : [];

  return (
    <>
      <TablePage
        id="reporting-library"
        title={t('page.library.title')}
        description={t('page.library.subtitle')}
        search={{
          value: query,
          onChange: setQuery,
          mode,
          onModeChange: setMode,
          placeholder: t('library.searchPlaceholder'),
          fields: (report) => [report.title, report.environment, report.headline, typeLabel(report.report_type)],
        }}
        savedFilters={{
          state: { type, query, mode },
          onRestore: (state) => {
            setType(typeof state.type === 'string' ? state.type : '');
            setQuery(typeof state.query === 'string' ? state.query : '');
            setMode((state.mode as SearchMode) ?? 'contains');
            setPage(1);
          },
        }}
        filters={
          <div className="w-56">
            <Select
              value={type}
              onChange={(value) => {
                setType(value);
                setPage(1);
              }}
              ariaLabel={t('library.filterByType')}
              options={[
                { value: '', label: t('library.allTypes') },
                ...(catalog.data?.types ?? []).map((x) => ({
                  value: x.key,
                  label: typeLabel(x.key),
                })),
              ]}
            />
          </div>
        }
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void reports.refetch(), isFetching: reports.isFetching },
          export: { filename: 'mcharbor-report-library' },
          chips: {
            items: chips,
            onClear: () => {
              setType('');
              setQuery('');
              setPage(1);
            },
          },
        }}
        columns={columns}
        data={items}
        loading={reports.isLoading}
        error={reports.error}
        onRetry={() => void reports.refetch()}
        empty={{
          icon: <IconArchive className="size-8" />,
          title: query.trim() || type ? t('library.noMatches') : t('library.empty.title'),
          description: query.trim() || type ? undefined : t('library.empty.description'),
        }}
        getRowId={(report) => report.id}
        onRowClick={(report) => onOpen(report.id)}
        pagination={{
          mode: 'server',
          page,
          totalPages,
          pageSize,
          total,
          from,
          to,
          onPageChange: (next) => setPage(Math.min(Math.max(next, 1), totalPages)),
          onPageSizeChange: (next) => {
            setPageSize(next);
            setPage(1);
          },
        }}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setPendingDelete(null);
        }}
        title={t('library.delete.title')}
        description={t('library.delete.description', { title: pendingDelete?.title ?? '' })}
        confirmLabel={t('action.delete')}
        variant="destructive"
        loading={remove.isPending}
        onConfirm={() => {
          if (pendingDelete) remove.mutate(pendingDelete.id);
          setPendingDelete(null);
        }}
      />
    </>
  );
}
