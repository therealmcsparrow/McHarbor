// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconEyeOff, IconSparkles } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { formatBytes, timeAgo } from '@resources/utils/format';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PERM } from '@core/auth/permissionKeys';
import { RefusalAlert } from '../components/RefusalAlert';
import { type Finding, useDismissFinding, useExplainFinding, useFindings } from '../hooks/useNarrationSettings';

function firstNumber(f: Finding): number | null {
  return Object.values(numbers(f)).find((v): v is number => typeof v === 'number') ?? null;
}

function numbers(f: Finding): Record<string, string | number> {
  const m = f.metrics;
  switch (f.kind) {
    case 'memory_climb':
      return {
        growth: m.growthPercent ?? 0,
        days: m.days ?? 0,
        perDay: formatBytes(m.bytesPerDay ?? 0),
        r2: m.r2 ?? 0,
      };
    case 'host_memory_forecast':
      return { current: m.currentPercent ?? 0, days: m.daysUntilFull ?? 0, perDay: m.percentPerDay ?? 0 };
    case 'scheduled_restarts':
      return {
        time: `${String(m.hour ?? 0).padStart(2, '0')}:${String(m.minute ?? 0).padStart(2, '0')}`,
        days: m.days ?? 0,
      };
    case 'post_deploy_failures':
      return { before: m.failuresBefore ?? 0, after: m.failuresAfter ?? 0 };
    case 'host_disk_forecast':
    case 'storage_location_forecast':
      return {
        current: m.currentPercent ?? 0,
        days: m.daysUntilFull ?? 0,
        perDay: m.percentPerDay ?? 0,
        ceiling: m.ceilingPercent ?? 0,
        r2: m.r2 ?? 0,
      };
  }
  return {};
}

export default function FindingsPage() {
  const { t } = useTranslation('incidents');
  const envId = useEnvironmentStore((s) => s.currentId);
  const findings = useFindings(envId, 'open');
  const explain = useExplainFinding();
  const dismiss = useDismissFinding();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [explainedEnvId, setExplainedEnvId] = useState<string | null>(null);

  const columns = useMemo<ColumnDef<Finding, unknown>[]>(
    () => [
      {
        accessorKey: 'kind',
        header: t('findings.columns.kind'),
        cell: ({ row }) => <Badge variant="info">{t(`findings.kind.${row.original.kind}`)}</Badge>,
      },
      {
        accessorKey: 'subjectName',
        header: t('findings.columns.subject'),
        cell: ({ row }) => <span className="font-medium">{row.original.subjectName}</span>,
      },
      {
        id: 'numbers',
        header: t('findings.columns.numbers'),
        meta: { sortValue: firstNumber },
        cell: ({ row }) => (
          <span className="text-sm">{t(`findings.says.${row.original.kind}`, numbers(row.original))}</span>
        ),
      },
      {
        id: 'explanation',
        header: t('findings.columns.explanation'),
        cell: ({ row }) =>
          row.original.explanation ? (
            <span className="block text-sm whitespace-normal">
              {row.original.explanation}
              <span className="mt-1 block text-xs text-muted-foreground">
                {t('findings.explainedBy', {
                  model: row.original.explanationMeta?.model ?? '',
                  who: row.original.explanationMeta?.requestedBy ?? '',
                })}
              </span>
            </span>
          ) : (
            '—'
          ),
      },
      {
        accessorKey: 'lastSeenAt',
        header: t('findings.columns.lastSeen'),
        cell: ({ row }) => timeAgo(row.original.lastSeenAt),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('findings.columns.actions')}</span>,
        meta: { locked: true, noExport: true, label: t('findings.columns.actions') },
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon"
              permission={PERM.INCIDENTS_MANAGE}
              aria-label={t('findings.explain')}
              title={t('findings.explain')}
              disabled={explain.isPending}
              onClick={() => {
                setExplainedEnvId(row.original.envId);
                explain.mutate(row.original.id);
              }}
            >
              <IconSparkles className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              permission={PERM.INCIDENTS_MANAGE}
              aria-label={t('findings.dismiss')}
              title={t('findings.dismiss')}
              disabled={dismiss.isPending}
              onClick={() => dismiss.mutate(row.original.id)}
            >
              <IconEyeOff className="h-4 w-4" />
            </Button>
          </div>
        ),
      },
    ],
    [t, explain, dismiss],
  );

  return (
    <>
      {explain.error && <RefusalAlert error={explain.error} envId={explainedEnvId} />}
      <TablePage
        id="incident-findings"
        title={t('findings.title')}
        description={t('findings.description')}
        actions={
          <Button asChild variant="outline">
            <Link to="/incidents">
              <IconArrowLeft className="h-4 w-4" /> {t('findings.back')}
            </Link>
          </Button>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('findings.columns.subject'),
          fields: (f) => [f.subjectName, f.kind],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void findings.refetch(), isFetching: findings.isFetching },
          export: { filename: 'mcharbor-behaviour-findings' },
        }}
        columns={columns}
        data={findings.data ?? []}
        loading={findings.isLoading}
        empty={{ title: t('findings.empty') }}
        getRowId={(f) => f.id}
      />
    </>
  );
}
