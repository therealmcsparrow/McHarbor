// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { IconCircleCheck, IconCircleX, IconLoader2, IconPlayerPlay } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { timeAgo } from '@resources/utils/format';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  IconTile,
  ROW_HEIGHT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';

type WorkflowRun = {
  id: string;
  workflowId?: string;
  workflowName: string;
  status: string;
  trigger: string;
  durationMs: number;
  startedAt: string;
};

function useWorkflowRuns(limit = 25) {
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

const STATUS: Record<string, { tone: Tone; icon: typeof IconCircleCheck }> = {
  success: { tone: 'success', icon: IconCircleCheck },
  completed: { tone: 'success', icon: IconCircleCheck },
  failed: { tone: 'danger', icon: IconCircleX },
  error: { tone: 'danger', icon: IconCircleX },
  running: { tone: 'info', icon: IconLoader2 },
};

const FALLBACK_STATUS: { tone: Tone; icon: typeof IconCircleCheck } = { tone: 'neutral', icon: IconPlayerPlay };

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
}

export default function WorkflowRunsWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: runs = [], isLoading, isError } = useWorkflowRuns();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const finished = runs.filter((r) => r.status !== 'running');
  const ok = finished.filter((r) => r.status === 'success' || r.status === 'completed').length;
  const percent = finished.length > 0 ? Math.round((ok / finished.length) * 100) : null;
  const shown = runs.slice(0, capacity);
  const remaining = runs.length - shown.length;

  return (
    <WidgetFrame
      title={t('workflowRunsWidget.title')}
      badge={
        percent !== null ? (
          <WidgetBadge tone={percent === 100 ? 'success' : percent < 50 ? 'danger' : 'warning'}>
            {t('workflowRunsWidget.successRate', { percent })}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('workflowRunsWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('workflowRunsWidget.viewAll')} onClick={() => navigate('/workflows')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : isError || runs.length === 0 ? (
        <WidgetEmpty
          icon={<IconPlayerPlay className="size-5" aria-hidden="true" />}
          title={t('workflowRunsWidget.noRuns')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((run) => {
            const s = STATUS[run.status] ?? FALLBACK_STATUS;
            const Icon = s.icon;
            return (
              <li key={run.id}>
                <WidgetRow
                  leading={
                    <IconTile tone={s.tone}>
                      <Icon
                        className={run.status === 'running' ? 'size-4 animate-spin' : 'size-4'}
                        aria-hidden="true"
                      />
                    </IconTile>
                  }
                  title={run.workflowName}
                  subtitle={`${run.trigger} · ${run.status}`}
                  trailing={
                    <>
                      <span className="tabular-nums">{formatDuration(run.durationMs)}</span>
                      <span className="w-12 text-right tabular-nums">{timeAgo(run.startedAt)}</span>
                    </>
                  }
                  onClick={() => navigate(run.workflowId ? `/workflows/${run.workflowId}/runs` : '/workflows')}
                />
              </li>
            );
          })}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
