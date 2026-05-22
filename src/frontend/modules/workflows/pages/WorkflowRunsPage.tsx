// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useNavigate } from 'react-router';
import type { ColumnDef } from '@tanstack/react-table';
import { IconArrowLeft, IconHistory } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { timeAgo } from '@resources/utils/format';
import { useWorkflow, useWorkflowRuns, type WorkflowRun } from '../hooks/useWorkflows';

const STATUS_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  completed: 'success',
  failed: 'destructive',
  cancelled: 'warning',
  running: 'secondary',
};

const TRIGGER_VARIANTS: Record<string, 'default' | 'secondary'> = {
  manual: 'default',
  auto: 'secondary',
};

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = (ms / 1000).toFixed(1);
  return `${seconds}s`;
}

export default function WorkflowRunsPage() {
  const { t } = useTranslation('common');
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: workflow } = useWorkflow(id ?? '');
  const { data: runs = [], isLoading } = useWorkflowRuns(id);

  const columns = useMemo<ColumnDef<WorkflowRun, unknown>[]>(
    () => [
      {
        accessorKey: 'status',
        header: t('workflows.columnStatus'),
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANTS[row.original.status] ?? 'secondary'}>
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'trigger',
        header: t('workflows.columnTrigger'),
        cell: ({ row }) => (
          <Badge variant={TRIGGER_VARIANTS[row.original.trigger] ?? 'secondary'}>
            {row.original.trigger}
          </Badge>
        ),
      },
      {
        accessorKey: 'nodeCount',
        header: t('workflows.columnNodes'),
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{row.original.nodeCount}</span>
        ),
      },
      {
        accessorKey: 'durationMs',
        header: t('workflows.columnDuration'),
        cell: ({ row }) => (
          <span className="font-mono text-sm text-muted-foreground">
            {formatDuration(row.original.durationMs)}
          </span>
        ),
      },
      {
        accessorKey: 'startedAt',
        header: t('workflows.columnStarted'),
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{timeAgo(row.original.startedAt)}</span>
        ),
      },
      {
        accessorKey: 'error',
        header: t('workflows.columnError'),
        cell: ({ row }) =>
          row.original.error ? (
            <span className="truncate text-sm text-destructive">{row.original.error}</span>
          ) : (
            <span className="text-sm text-muted-foreground">-</span>
          ),
      },
    ],
    [t],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => navigate('/workflows')}
                  aria-label={t('workflows.backToWorkflows')}
                  className="size-8"
                >
                  <IconArrowLeft className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('workflows.backToWorkflows')}</TooltipContent>
            </Tooltip>
            <div className="h-5 w-px bg-border" />
            <IconHistory className="size-5 text-muted-foreground" />
            <span>{t('workflows.runsTitle')}{workflow ? ` — ${workflow.name}` : ''}</span>
          </div>
        }
        description={t('workflows.runsCount', { count: runs.length })}
        actions={
          id ? (
            <Button variant="outline" onClick={() => navigate(`/workflows/${id}`)}>
              {t('workflows.openEditor')}
            </Button>
          ) : undefined
        }
      />
      <DataGrid
        data={runs}
        columns={columns}
        searchKey="status"
        searchPlaceholder={t('workflows.filterPlaceholder')}
        loading={isLoading}
        emptyMessage={t('workflows.noRuns')}
      />
    </div>
  );
}

