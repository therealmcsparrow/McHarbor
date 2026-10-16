// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconChartLine, IconSettings } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { timeAgo } from '@resources/utils/format';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PERM } from '@core/auth/permissionKeys';
import { type Incident, useIncidents } from '@resources/hooks/useIncidents';
import { useIncidentLabels } from '../hooks/useIncidentLabels';
import { NarrationSettingsDialog } from '../components/NarrationSettingsDialog';

export default function IncidentsPage() {
  const { t } = useTranslation('incidents');
  const labels = useIncidentLabels();
  const envId = useEnvironmentStore((s) => s.currentId);
  const [statusValue, setStatus] = useState<'open' | 'resolved' | ''>('open');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const incidents = useIncidents(statusValue);

  const columns = useMemo<ColumnDef<Incident, unknown>[]>(
    () => [
      {
        accessorKey: 'subjectName',
        header: t('columns.subject'),
        cell: ({ row }) => (
          <div className="min-w-0">
            <Link
              className="font-medium text-primary underline-offset-2 hover:underline"
              to={`/incidents/${encodeURIComponent(row.original.id)}`}
            >
              {row.original.subjectName}
            </Link>
            <p className="text-xs text-muted-foreground">{labels.subjectType(row.original)}</p>
          </div>
        ),
      },
      {
        accessorKey: 'signal',
        header: t('columns.signal'),
        cell: ({ row }) => <Badge variant="destructive">{labels.signal(row.original)}</Badge>,
      },
      { accessorKey: 'signalCount', header: t('columns.signals') },
      {
        accessorKey: 'status',
        header: t('columns.status'),
        cell: ({ row }) => (
          <Badge variant={row.original.status === 'open' ? 'warning' : 'success'}>
            {t(`status.${row.original.status}`)}
          </Badge>
        ),
      },
      {
        accessorKey: 'narrativeStatus',
        header: t('columns.narrative'),
        cell: ({ row }) => t(`narrativeStatus.${row.original.narrativeStatus}`),
      },
      {
        accessorKey: 'firstSignalAt',
        header: t('columns.started'),
        cell: ({ row }) => timeAgo(row.original.firstSignalAt),
      },
      { accessorKey: 'lastSignalAt', header: t('columns.last'), cell: ({ row }) => timeAgo(row.original.lastSignalAt) },
    ],
    [t, labels],
  );

  return (
    <>
      <TablePage
        id="incidents"
        title={t('title')}
        description={t('description')}
        actions={
          <div className="flex items-center gap-2">
            <Select
              value={statusValue}
              onChange={(v) => setStatus(v as 'open' | 'resolved' | '')}
              ariaLabel={t('columns.status')}
              options={[
                { value: 'open', label: t('status.open') },
                { value: 'resolved', label: t('status.resolved') },
                { value: '', label: t('status.all') },
              ]}
            />
            <Button asChild variant="outline">
              <Link to="/incidents/findings">
                <IconChartLine className="h-4 w-4" /> {t('findings.open')}
              </Link>
            </Button>
            <Button
              variant="outline"
              permission={PERM.INCIDENTS_VIEW}
              disabled={!envId}
              onClick={() => setSettingsOpen(true)}
            >
              <IconSettings className="h-4 w-4" /> {t('settings.open')}
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('columns.container'),
          fields: (i) => [i.subjectName, i.stackName ?? '', i.signal],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void incidents.refetch(), isFetching: incidents.isFetching },
          export: { filename: 'mcharbor-incidents' },
        }}
        columns={columns}
        data={incidents.data ?? []}
        loading={incidents.isLoading}
        empty={{ title: t('empty') }}
        getRowId={(i) => i.id}
      />
      {settingsOpen && envId && <NarrationSettingsDialog envId={envId} onOpenChange={setSettingsOpen} />}
    </>
  );
}
