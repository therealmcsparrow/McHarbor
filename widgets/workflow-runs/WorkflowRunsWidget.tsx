// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { timeAgo } from '@resources/utils/format';
import { Badge } from '@resources/components/ui/Badge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type WorkflowRun = {
  id: string;
  workflowName: string;
  status: string;
  trigger: string;
  durationMs: number;
  startedAt: string;
};

function useWorkflowRuns(limit = 8) {
  return useQuery({
    queryKey: ['workflow-runs', limit],
    queryFn: () =>
      api
        .get<WorkflowRun[]>('/workflows/runs', { limit: String(limit) })
        .then((r) => (Array.isArray(r.data) ? r.data : [])),
    refetchInterval: 15_000,
    retry: false,
  });
}

const STATUS_VARIANT: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  success: 'success',
  completed: 'success',
  failed: 'destructive',
  error: 'destructive',
  running: 'warning',
};

export default function WorkflowRunsWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: runs, isLoading, isError } = useWorkflowRuns();

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (isError || !runs || runs.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('workflowRunsWidget.noRuns')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">
        {t('workflowRunsWidget.title')}
      </h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="pb-1.5 font-medium">{t('workflowRunsWidget.workflow')}</th>
              <th className="pb-1.5 font-medium">{t('workflowRunsWidget.status')}</th>
              <th className="pb-1.5 font-medium">{t('workflowRunsWidget.duration')}</th>
              <th className="pb-1.5 font-medium">{t('workflowRunsWidget.when')}</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.id} className="border-b border-border/50 last:border-0">
                <td className="max-w-[120px] truncate py-1.5 pr-2 text-foreground">
                  {run.workflowName}
                </td>
                <td className="py-1.5 pr-2">
                  <Badge variant={STATUS_VARIANT[run.status] ?? 'secondary'} className="text-[10px] px-1.5 py-0.5">
                    {run.status}
                  </Badge>
                </td>
                <td className="py-1.5 pr-2 text-muted-foreground">
                  {run.durationMs < 1000 ? `${run.durationMs}ms` : `${(run.durationMs / 1000).toFixed(1)}s`}
                </td>
                <td className="py-1.5 text-muted-foreground">{timeAgo(run.startedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
