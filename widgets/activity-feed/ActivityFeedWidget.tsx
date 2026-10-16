// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { IconActivity } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  ROW_HEIGHT,
  ToneDot,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  relativeTime,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';

// Mirrors modules/activity/service.go.
type ActivityEvent = {
  id: string;
  containerId: string;
  containerName?: string;
  eventType: string;
  action: string;
  timestamp: string;
};

const ACTION_TONE: Record<string, Tone> = {
  start: 'success',
  create: 'success',
  unpause: 'success',
  stop: 'danger',
  die: 'danger',
  kill: 'danger',
  destroy: 'danger',
  restart: 'warning',
  pause: 'warning',
};

function actionTone(action: string): Tone {
  return ACTION_TONE[action] ?? 'neutral';
}

function useActivityFeed() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['activity-feed-widget', envId],
    queryFn: () =>
      api
        .get<PaginatedData<ActivityEvent>>('/activity', {
          ...(envId ? { env: envId } : {}),
          per_page: '25',
        })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 10_000,
  });
}

export default function ActivityFeedWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t, i18n } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: raw = [], isLoading } = useActivityFeed();
  // The collector records every Docker event twice; one row per event is enough.
  const events = raw.filter(
    (e, i) =>
      i === 0 ||
      !(
        raw[i - 1]?.containerId === e.containerId &&
        raw[i - 1]?.action === e.action &&
        raw[i - 1]?.timestamp === e.timestamp
      ),
  );
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const shown = events.slice(0, capacity);
  const remaining = events.length - shown.length;

  return (
    <WidgetFrame
      title={t('activityFeedWidget.title')}
      badge={events.length > 0 ? <WidgetBadge>{events.length}</WidgetBadge> : undefined}
      footer={
        <>
          <span>{remaining > 0 ? t('activityFeedWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('activityFeedWidget.viewAll')} onClick={() => navigate('/activity')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('activityFeedWidget.loadingActivity')} />
      ) : events.length === 0 ? (
        <WidgetEmpty
          icon={<IconActivity className="size-5" aria-hidden="true" />}
          title={t('activityFeedWidget.noRecentActivity')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((entry) => {
            const tone = actionTone(entry.action);
            return (
              <li key={entry.id}>
                <WidgetRow
                  leading={<ToneDot tone={tone} />}
                  title={entry.containerName || entry.containerId.slice(0, 12)}
                  subtitle={entry.eventType}
                  trailing={
                    <span className="flex flex-col items-end gap-0.5">
                      <span className="rounded-full bg-glass-2 px-1.5 py-0.5 text-[10px] leading-none font-medium tracking-wide text-foreground uppercase">
                        {entry.action}
                      </span>
                      <span className="text-[11px] leading-none whitespace-nowrap tabular-nums">
                        {relativeTime(entry.timestamp, t('activityFeedWidget.justNow'), i18n.language)}
                      </span>
                    </span>
                  }
                />
              </li>
            );
          })}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
