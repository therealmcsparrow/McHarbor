// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconCloudOff,
  IconPlayerPlay,
  IconPlayerStop,
  IconSortAscending,
  IconSortDescending,
} from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { textColumn } from '@resources/layout/table-page/optional-columns';
import { useAgentLogs, type AgentLogEntry } from '../hooks/useAgentLogs';

const PAGE_SIZE = 25;

type AgentLogCardProps = {
  envId: string;
};

type BadgeVariant = 'secondary' | 'default' | 'warning' | 'destructive';

function levelVariant(level: string): BadgeVariant {
  switch (level) {
    case 'error':
      return 'destructive';
    case 'warn':
    case 'warning':
      return 'warning';
    case 'debug':
      return 'secondary';
    default:
      return 'default';
  }
}

function attrText(attrs: AgentLogEntry['attrs']): string {
  if (!attrs) return '';
  return Object.entries(attrs)
    .map(([key, value]) => `${key}=${typeof value === 'string' ? value : JSON.stringify(value)}`)
    .join(' ');
}

/**
 * The agent's own log lines, replayed from the local buffer it kept while it
 * could not reach McHarbor. Shown on the environment overview under the agent
 * info card so an operator who sees "last seen 3 hours ago" can read what the
 * agent was doing in the gap.
 */
export function AgentLogCard({ envId }: AgentLogCardProps) {
  const { t } = useTranslation('environments');
  const { t: tc } = useTranslation('containers');
  const prefs = useLocaleFormat();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE);
  const [live, setLive] = useState(false);
  const [newestFirst, setNewestFirst] = useState(true);

  const { data, isPending, isFetching, refetch } = useAgentLogs(envId, page, pageSize, true, live);
  const group = t('columns.groupLogEntry');

  const total = data?.total ?? 0;
  const totalPages = data?.total_pages ?? 0;

  const rows = useMemo(() => {
    const items = data?.items ?? [];
    return newestFirst ? items : [...items].reverse();
  }, [data?.items, newestFirst]);

  const columns = useMemo<ColumnDef<AgentLogEntry, unknown>[]>(
    () => [
      {
        id: 'time',
        header: t('detail.agentLog.columnTime'),
        accessorFn: (row) => row.loggedAt,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{formatDate(row.original.loggedAt, prefs)}</span>
        ),
      },
      {
        id: 'level',
        header: t('detail.agentLog.columnLevel'),
        accessorFn: (row) => row.level,
        cell: ({ row }) => (
          <Badge variant={levelVariant(row.original.level)} className="px-2 py-0.5 uppercase">
            {row.original.level}
          </Badge>
        ),
      },
      {
        id: 'message',
        header: t('detail.agentLog.columnMessage'),
        accessorFn: (row) => `${row.message} ${attrText(row.attrs)}`,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="break-words text-foreground">{row.original.message}</p>
            {row.original.attrs && (
              <p className="font-mono text-xs break-all text-muted-foreground">{attrText(row.original.attrs)}</p>
            )}
          </div>
        ),
      },
      textColumn<AgentLogEntry>('attrs', t('columns.attrs'), (r) => (r.attrs ? JSON.stringify(r.attrs) : null), {
        group,
        width: 'xl',
      }),
      textColumn<AgentLogEntry>('entryId', t('columns.entryId'), (r) => r.id, { group }),
    ],
    [prefs, t, group],
  );

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4">
        <h3 className="text-sm font-medium text-foreground">{t('detail.agentLog.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('detail.agentLog.description')}</p>
      </div>

      <div className="h-[26rem]">
        <TablePage
          id="environment-agent-log"
          header={false}
          title={t('detail.agentLog.title')}
          columns={columns}
          data={rows}
          loading={isPending}
          getRowId={(row) => row.id}
          empty={{
            icon: <IconCloudOff className="h-5 w-5" />,
            title: t('detail.agentLog.emptyTitle'),
            description: t('detail.agentLog.emptyDescription'),
          }}
          toolbar={{
            columns: true,
            density: true,
            refresh: { onRefresh: () => void refetch(), isFetching },
            export: { filename: t('detail.agentLog.exportName', { env: envId }) },
            actions: (
              <>
                <Button
                  variant={live ? 'default' : 'outline'}
                  size="sm"
                  aria-pressed={live}
                  onClick={() => setLive(!live)}
                >
                  {live ? <IconPlayerStop className="size-4" /> : <IconPlayerPlay className="size-4" />}{' '}
                  {tc('logs.live')}
                </Button>
                <Button variant="outline" size="sm" onClick={() => setNewestFirst(!newestFirst)}>
                  {newestFirst ? <IconSortDescending className="size-4" /> : <IconSortAscending className="size-4" />}
                  {newestFirst ? tc('logs.newestFirst') : tc('logs.oldestFirst')}
                </Button>
              </>
            ),
          }}
          pagination={{
            mode: 'server',
            page,
            totalPages,
            pageSize,
            total,
            from: total === 0 ? 0 : (page - 1) * pageSize + 1,
            to: Math.min(page * pageSize, total),
            onPageChange: setPage,
            onPageSizeChange: (next) => {
              setPageSize(next);
              setPage(1);
            },
          }}
        />
      </div>
    </div>
  );
}
