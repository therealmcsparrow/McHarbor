// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconHistory } from '@tabler/icons-react';
import { useRecentLifecycleEvents } from '@modules/lifecycle/hooks/useLifecycle';
import type { LifecycleSeverity } from '@modules/lifecycle/hooks/useLifecycle';
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

/**
 * Recent lifecycle events across every resource type.
 *
 * Distinct from the Activity Feed beside it: that one streams raw Docker
 * container events, while this reads the lifecycle store, which records
 * create / update / delete across containers, images, volumes, networks and
 * stacks — including changes McHarbor itself made rather than only what the
 * daemon emitted.
 *
 * `GET /lifecycle/recent` and `useRecentLifecycleEvents` both already existed;
 * nothing rendered them, which is the only reason the hook read as orphaned.
 */
const SEVERITY_TONE: Record<LifecycleSeverity, Tone> = {
  success: 'success',
  error: 'danger',
  warning: 'warning',
  info: 'neutral',
};

export default function LifecycleFeedWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t, i18n } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data, isLoading } = useRecentLifecycleEvents(25);
  const events = data?.items ?? [];
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const shown = events.slice(0, capacity);
  const remaining = events.length - shown.length;

  return (
    <WidgetFrame
      title={t('lifecycleFeedWidget.title')}
      badge={events.length > 0 ? <WidgetBadge>{events.length}</WidgetBadge> : undefined}
      footer={
        <>
          <span>{remaining > 0 ? t('lifecycleFeedWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('lifecycleFeedWidget.viewAll')} onClick={() => navigate('/lifecycle')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('lifecycleFeedWidget.loading')} />
      ) : events.length === 0 ? (
        <WidgetEmpty
          icon={<IconHistory className="size-5" aria-hidden="true" />}
          title={t('lifecycleFeedWidget.empty')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((entry) => (
            <li key={entry.id}>
              <WidgetRow
                leading={<ToneDot tone={SEVERITY_TONE[entry.severity] ?? 'neutral'} />}
                title={entry.subjectName || entry.subjectId.slice(0, 12)}
                subtitle={entry.subjectType}
                trailing={
                  <span className="flex flex-col items-end gap-0.5">
                    <span className="rounded-full bg-glass-2 px-1.5 py-0.5 text-[10px] leading-none font-medium tracking-wide text-foreground uppercase">
                      {entry.action}
                    </span>
                    <span className="text-[11px] leading-none whitespace-nowrap tabular-nums">
                      {relativeTime(entry.timestamp, t('lifecycleFeedWidget.justNow'), i18n.language)}
                    </span>
                  </span>
                }
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
