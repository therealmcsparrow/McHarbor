// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { StatusBadge } from './StatusBadge';
import { TaskLogsDialog } from './TaskLogsDialog';
import { useSwarmTasks, type SwarmTaskFilter } from '../hooks/useSwarmTasks';
import { taskStateTone } from '../format';
import type { SwarmTask } from '../types';

type SwarmTaskTableProps = {
  /** Server-side narrowing. Empty means the whole cluster. */
  filter?: SwarmTaskFilter;
  title: string;
  description?: string;
  /** Distinct per mount point so column widths and density do not collide. */
  id: string;
  /** Hide columns the surrounding page already answers. */
  hide?: { service?: boolean; node?: boolean };
  /** Rendered without the page chrome when embedded in a detail tab. */
  header?: boolean;
};

/**
 * The one task table.
 *
 * Three pages want a list of tasks — the cluster-wide page, a service's tasks
 * tab and a node's — and they differ only in which column is redundant and
 * what the filter is. Three copies of this would be three places to fix the
 * next time a task state is added.
 */
export function SwarmTaskTable({ filter = {}, title, description, id, hide, header = true }: SwarmTaskTableProps) {
  const { t } = useTranslation('swarm');
  const { data: tasks = [], isLoading, isFetching, refetch } = useSwarmTasks(filter);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [logsTask, setLogsTask] = useState<SwarmTask | null>(null);

  const columns = useMemo<ColumnDef<SwarmTask, unknown>[]>(() => {
    const cols: ColumnDef<SwarmTask, unknown>[] = [
      {
        accessorKey: 'name',
        header: t('columns.task'),
        meta: { flex: true },
        cell: ({ row }) => <span className="truncate font-medium">{row.original.name}</span>,
      },
    ];

    if (!hide?.service) {
      cols.push({
        accessorKey: 'serviceName',
        header: t('columns.service'),
        size: 160,
        cell: ({ row }) => (
          <span className="truncate text-sm text-muted-foreground">{row.original.serviceName || '—'}</span>
        ),
      });
    }

    if (!hide?.node) {
      cols.push({
        accessorKey: 'nodeName',
        header: t('columns.node'),
        size: 150,
        cell: ({ row }) => (
          <span className="truncate text-sm text-muted-foreground">{row.original.nodeName || '—'}</span>
        ),
      });
    }

    cols.push(
      {
        accessorKey: 'desiredState',
        header: t('columns.desiredState'),
        size: 120,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{t(`taskState.${row.original.desiredState}`)}</span>
        ),
      },
      {
        accessorKey: 'state',
        header: t('columns.currentState'),
        size: 130,
        cell: ({ row }) => (
          <StatusBadge tone={taskStateTone(row.original.state)}>{t(`taskState.${row.original.state}`)}</StatusBadge>
        ),
      },
      {
        id: 'error',
        header: t('columns.message'),
        meta: { flex: true, sortValue: (task: SwarmTask) => task.error || task.message || '' },
        cell: ({ row }) => {
          // The error is the reason a task is not running and is the single
          // most useful thing on this row when one is not; it wins over the
          // routine status message.
          const text = row.original.error || row.original.message;
          return text ? (
            <span className="truncate text-xs text-muted-foreground" title={text}>
              {text}
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          );
        },
      },
    );

    return cols;
  }, [t, hide?.service, hide?.node]);

  return (
    <>
      <TablePage
        id={id}
        header={header}
        title={title}
        description={description}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('tasks.searchPlaceholder'),
          fields: (task: SwarmTask) => [task.name, task.serviceName, task.nodeName, task.state, task.error],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-swarm-tasks' },
        }}
        columns={columns}
        data={tasks}
        loading={isLoading}
        empty={{ title: t('tasks.empty') }}
        getRowId={(row) => row.id}
        // A task has no page of its own — it is a runtime instance, not a
        // configured object — so the row opens its logs, which is the only
        // thing anybody wants from one.
        onRowClick={(row) => setLogsTask(row)}
      />

      <TaskLogsDialog task={logsTask} onOpenChange={() => setLogsTask(null)} />
    </>
  );
}
