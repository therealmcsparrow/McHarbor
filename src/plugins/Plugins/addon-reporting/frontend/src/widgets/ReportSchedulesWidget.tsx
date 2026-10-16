// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// report-schedules — every schedule with its last outcome and next run,
// failures first. A schedule that quietly stopped delivering is the thing
// this tile exists to surface.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconCalendarOff, IconCalendarTime, IconCircleCheck, IconCircleX, IconLoader2 } from '@tabler/icons-react';
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
  relativeTime,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import * as api from '../api';
import type { Schedule } from '../types';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH } from './RecentReportsWidget';

/** `timeAgo` clamps the future to "now"; a next run wants "in 3 hours". */
function timeUntil(iso: string): string {
  const diff = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  if (!Number.isFinite(diff)) return '';
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
  if (diff < 60) return rtf.format(0, 'second');
  if (diff < 3600) return rtf.format(Math.round(diff / 60), 'minute');
  if (diff < 86400) return rtf.format(Math.round(diff / 3600), 'hour');
  return rtf.format(Math.round(diff / 86400), 'day');
}

function outcome(s: Schedule): { tone: Tone; icon: typeof IconCircleCheck; rank: number } {
  if (!s.enabled) return { tone: 'neutral', icon: IconCalendarOff, rank: 3 };
  switch (s.last_run?.status) {
    case 'failed':
      return { tone: 'danger', icon: IconCircleX, rank: 0 };
    case 'partial':
      return { tone: 'warning', icon: IconCircleX, rank: 1 };
    case 'running':
      return { tone: 'info', icon: IconLoader2, rank: 2 };
    case 'success':
      return { tone: 'success', icon: IconCircleCheck, rank: 2 };
    default:
      return { tone: 'neutral', icon: IconCalendarTime, rank: 2 };
  }
}

export function ReportSchedulesWidget() {
  const { t, i18n } = useTranslation('reporting');
  const ago = (iso: string) => relativeTime(iso, t('widgets.common.justNow'), i18n.language);
  const { navigate } = useWidgetContext();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const schedules = useQuery({
    queryKey: ['reporting', 'widget', 'schedules'],
    queryFn: () => api.fetchSchedules(1, 50),
    refetchInterval: 60_000,
  });

  const rows = useMemo(() => {
    return (schedules.data?.items ?? [])
      .map((s) => ({ ...s, ...outcome(s) }))
      .sort((a, b) => a.rank - b.rank || a.name.localeCompare(b.name));
  }, [schedules.data]);

  const failing = rows.filter((r) => r.tone === 'danger' || r.tone === 'warning').length;
  const enabled = rows.filter((r) => r.enabled).length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.reportSchedules.title')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge tone={failing > 0 ? 'danger' : 'success'}>
            {failing > 0
              ? t('widgets.reportSchedules.failing', { count: failing })
              : t('widgets.reportSchedules.enabled', { count: enabled })}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('menu.schedules')} onClick={() => navigate(`${PLUGIN_PATH}/schedules`)} />
        </>
      }
    >
      {schedules.isLoading ? (
        <WidgetLoading label={t('widgets.common.loading')} />
      ) : schedules.isError ? (
        <WidgetEmpty
          icon={<IconCalendarOff className="size-5" aria-hidden="true" />}
          title={t('widgets.common.unavailable')}
        />
      ) : rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconCalendarTime className="size-5" aria-hidden="true" />}
          title={t('widgets.reportSchedules.none')}
          hint={t('widgets.reportSchedules.noneHint')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((s) => (
            <li key={s.id}>
              <WidgetRow
                leading={
                  <IconTile tone={s.tone}>
                    <s.icon
                      className={s.last_run?.status === 'running' ? 'size-4 animate-spin' : 'size-4'}
                      aria-hidden="true"
                    />
                  </IconTile>
                }
                title={s.name}
                subtitle={
                  s.last_run
                    ? t('widgets.reportSchedules.lastRun', { when: ago(s.last_run.started_at) })
                    : t('widgets.reportSchedules.neverRan')
                }
                trailing={
                  <span className="tabular-nums">
                    {s.enabled
                      ? s.next_run_at
                        ? t('widgets.reportSchedules.next', { when: timeUntil(s.next_run_at) })
                        : ''
                      : t('widgets.reportSchedules.paused')}
                  </span>
                }
                onClick={() => navigate(`${PLUGIN_PATH}/schedules`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
