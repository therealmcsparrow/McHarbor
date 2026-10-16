// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { IconStack2 } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { StatusBadge, STACK_STATUS } from '@resources/components/ui/StatusBadge';
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

type StackSummary = {
  id: string;
  name: string;
  status: string;
  serviceCount: number;
};

const STATUS_TONE: Record<string, Tone> = {
  running: 'success',
  partial: 'warning',
  degraded: 'warning',
  stopped: 'neutral',
  error: 'danger',
  failed: 'danger',
};

const STATUS_RANK: Record<string, number> = { error: 0, failed: 0, partial: 1, degraded: 1, running: 2, stopped: 3 };

function useStacks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stacks-widget', envId],
    queryFn: () =>
      api
        .get<PaginatedData<StackSummary>>('/stacks', { ...(envId ? { env: envId } : {}), per_page: '100' })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 15_000,
  });
}

export default function StackStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: stacks = [], isLoading } = useStacks();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const sorted = [...stacks].sort(
    (a, b) => (STATUS_RANK[a.status] ?? 9) - (STATUS_RANK[b.status] ?? 9) || a.name.localeCompare(b.name),
  );
  const running = stacks.filter((s) => s.status === 'running').length;
  const shown = sorted.slice(0, capacity);
  const remaining = sorted.length - shown.length;

  return (
    <WidgetFrame
      title={t('stackStatusWidget.title')}
      badge={
        stacks.length > 0 ? (
          <WidgetBadge tone={running === stacks.length ? 'success' : 'neutral'}>
            {t('stackStatusWidget.runningOf', { running, total: stacks.length })}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('stackStatusWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('stackStatusWidget.viewAll')} onClick={() => navigate('/stacks')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('stackStatusWidget.loadingStacks')} />
      ) : stacks.length === 0 ? (
        <WidgetEmpty
          icon={<IconStack2 className="size-5" aria-hidden="true" />}
          title={t('stackStatusWidget.noStacksFound')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((stack) => (
            <li key={stack.id}>
              <WidgetRow
                leading={
                  <IconTile tone={STATUS_TONE[stack.status] ?? 'neutral'}>
                    <IconStack2 className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={stack.name}
                subtitle={t('stackStatusWidget.service', { count: stack.serviceCount })}
                trailing={
                  <StatusBadge status={stack.status} map={STACK_STATUS} className="px-1.5 py-0.5 text-[10px]" />
                }
                onClick={() => navigate(`/stacks/${encodeURIComponent(stack.name)}`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
