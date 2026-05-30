// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Badge } from '@resources/components/ui/Badge';
import { timeAgo } from '@resources/utils/format';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type ActivityEvent = {
  id: string;
  type: string;
  action: string;
  resourceType: string;
  resourceName: string;
  timestamp: string;
};

const ACTION_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  start: 'success',
  create: 'success',
  stop: 'destructive',
  die: 'destructive',
  kill: 'destructive',
  destroy: 'destructive',
  restart: 'warning',
  pause: 'warning',
  unpause: 'success',
};

function useActivityFeed() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['activity-feed-widget', envId],
    queryFn: () =>
      api
        .get<PaginatedData<ActivityEvent>>('/activity', {
          ...(envId ? { env: envId } : {}),
          per_page: '15',
        })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 10_000,
  });
}

export default function ActivityFeedWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: events, isLoading } = useActivityFeed();

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('activityFeedWidget.loadingActivity')}
      </div>
    );
  }

  if (!events || events.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('activityFeedWidget.noRecentActivity')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">{t('activityFeedWidget.title')}</h3>
      <div className="flex-1 overflow-y-auto px-4 pb-3">
        <div className="space-y-2">
          {events.map((event) => (
            <div key={event.id} className="flex items-center gap-2 text-xs">
              <Badge variant={ACTION_VARIANTS[event.action] ?? 'secondary'} className="shrink-0 text-[10px] px-1.5 py-0.5">
                {event.action}
              </Badge>
              <span className="min-w-0 truncate text-foreground">{event.resourceName || event.resourceType}</span>
              <span className="ml-auto shrink-0 text-muted-foreground">{timeAgo(event.timestamp)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
