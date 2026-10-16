// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useKindLabel } from '../hooks/usePluginKinds';
import { IconArrowLeft } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { timeAgo } from '@resources/utils/format';
import { useDriftReport, type DriftReportRow } from '../hooks/useDriftReport';
import type { DriftClass } from '../hooks/useReconciler';

const CLASSES: DriftClass[] = ['configuration', 'state', 'existence', 'image'];

export default function DriftReportPage() {
  const { t } = useTranslation('reconciler');
  const kindLabel = useKindLabel();
  const [driftClass, setDriftClass] = useState<DriftClass | ''>('');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const report = useDriftReport(driftClass);

  const columns = useMemo<ColumnDef<DriftReportRow, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('report.columns.watch'),
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <span className="font-medium">{row.original.name}</span>
            <Badge variant="secondary">{kindLabel(row.original.kind)}</Badge>
          </span>
        ),
      },
      {
        accessorKey: 'class',
        header: t('report.columns.class'),
        cell: ({ row }) => <Badge variant="warning">{t(`class.${row.original.class}`)}</Badge>,
      },
      {
        accessorKey: 'field',
        header: t('report.columns.field'),
        cell: ({ row }) => <span className="font-mono text-xs break-all">{row.original.field}</span>,
      },
      {
        accessorKey: 'expected',
        header: t('report.columns.expected'),
        cell: ({ row }) => <span className="font-mono text-xs break-all">{row.original.expected}</span>,
      },
      {
        accessorKey: 'actual',
        header: t('report.columns.actual'),
        cell: ({ row }) => <span className="font-mono text-xs break-all">{row.original.actual}</span>,
      },
      {
        accessorKey: 'firstSeenAt',
        header: t('report.columns.firstSeen'),
        cell: ({ row }) => (row.original.firstSeenAt ? timeAgo(row.original.firstSeenAt) : '—'),
      },
      {
        accessorKey: 'checkedAt',
        header: t('report.columns.checked'),
        cell: ({ row }) => (row.original.checkedAt ? timeAgo(row.original.checkedAt) : '—'),
      },
    ],
    [t, kindLabel],
  );

  return (
    <TablePage
      id="reconciler-report"
      title={t('report.title')}
      description={t('report.description')}
      actions={
        <div className="flex items-center gap-2">
          <Select
            value={driftClass}
            onChange={(value) => setDriftClass(value as DriftClass | '')}
            ariaLabel={t('report.classFilter')}
            options={[
              { value: '', label: t('report.allClasses') },
              ...CLASSES.map((c) => ({ value: c, label: t(`class.${c}`) })),
            ]}
          />
          <Button asChild variant="outline">
            <Link to="/reconciler">
              <IconArrowLeft className="h-4 w-4" /> {t('report.back')}
            </Link>
          </Button>
        </div>
      }
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('report.columns.field'),
        fields: (row) => [row.name, row.field, row.expected, row.actual],
      }}
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void report.refetch(), isFetching: report.isFetching },
        export: { filename: 'mcharbor-drift-report' },
      }}
      columns={columns}
      data={report.data ?? []}
      loading={report.isLoading}
      empty={{ title: t('report.empty') }}
      getRowId={(row) => `${row.desiredStateId}|${row.class}|${row.field}`}
    />
  );
}
