// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useUnreadInAppNotificationCount } from '@resources/hooks/useInAppNotifications';
import type { SidebarItemBadgeData } from './SidebarItemBadge';

type DashboardCounts = {
  containers: { total: number; running: number; stopped: number; paused: number };
  images: number;
  volumes: number;
  networks: number;
  stacks: number;
};

type ScanSummary = {
  critical: number;
  high: number;
  medium: number;
  low: number;
};

export function useSidebarStatusBadges(visiblePaths: readonly string[]): Record<string, SidebarItemBadgeData> {
  const { t } = useTranslation('common');
  const envId = useEnvironmentStore((s) => s.currentId);
  const envParams: Record<string, string> | undefined = envId ? { env: envId } : undefined;
  const visible = useMemo(() => new Set(visiblePaths), [visiblePaths]);
  const showDockerCounts =
    visible.has('/containers') ||
    visible.has('/images') ||
    visible.has('/volumes') ||
    visible.has('/networks') ||
    visible.has('/stacks');

  const dashboard = useQuery({
    queryKey: ['sidebar-status', 'dashboard', envId],
    queryFn: () => api.get<DashboardCounts>('/dashboard/stats', envParams).then(assertSuccess),
    enabled: showDockerCounts,
    refetchInterval: 15_000,
  });

  const scans = useQuery({
    queryKey: ['sidebar-status', 'scans', envId],
    queryFn: () => api.get<ScanSummary>('/scans/summary', envParams).then(assertSuccess),
    enabled: visible.has('/security/scans'),
    refetchInterval: 30_000,
  });

  const unreadNotifications = useUnreadInAppNotificationCount();

  return useMemo(() => {
    const badges: Record<string, SidebarItemBadgeData> = {};
    const stats = dashboard.data;

    if (stats && showDockerCounts) {
      const down = stats.containers.stopped + stats.containers.paused;
      if (visible.has('/containers')) {
        badges['/containers'] = {
          value: String(down > 0 ? down : stats.containers.running),
          label: t(down > 0 ? 'nav.badges.containersDown' : 'nav.badges.containersRunning', {
            count: down > 0 ? down : stats.containers.running,
          }),
          tone: down > 0 ? 'warning' : 'success',
        };
      }
      if (visible.has('/images')) {
        badges['/images'] = countBadge(stats.images, t('nav.badges.images', { count: stats.images }));
      }
      if (visible.has('/volumes')) {
        badges['/volumes'] = countBadge(stats.volumes, t('nav.badges.volumes', { count: stats.volumes }));
      }
      if (visible.has('/networks')) {
        badges['/networks'] = countBadge(stats.networks, t('nav.badges.networks', { count: stats.networks }));
      }
      if (visible.has('/stacks')) {
        badges['/stacks'] = countBadge(stats.stacks, t('nav.badges.stacks', { count: stats.stacks }));
      }
    }

    if (scans.data && visible.has('/security/scans')) {
      const urgent = scans.data.critical + scans.data.high;
      if (urgent > 0) {
        badges['/security/scans'] = {
          value: String(urgent),
          label: t('nav.badges.securityScans', { count: urgent }),
          tone: scans.data.critical > 0 ? 'destructive' : 'warning',
        };
      }
    }

    if ((unreadNotifications.data ?? 0) > 0 && visible.has('/notifications')) {
      badges['/notifications'] = {
        value: String(unreadNotifications.data),
        label: t('nav.badges.notifications', {
          count: unreadNotifications.data,
        }),
        tone: 'default',
      };
    }

    return badges;
  }, [dashboard.data, scans.data, showDockerCounts, t, unreadNotifications.data, visible]);
}

function countBadge(count: number, label: string): SidebarItemBadgeData {
  return {
    value: String(count),
    label,
    tone: 'secondary',
  };
}
