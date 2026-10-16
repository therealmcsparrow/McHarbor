// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The lifecycle log's timeline rendering. It carries the same five columns the
// table does: time (rail + date group, with the source tooltip), severity
// (marker tone + badge), subject (icon, link, type and environment), the
// action/state detail line, and the eye button that opens the details dialog.

import { useMemo } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconHistory } from '@tabler/icons-react';
import { EventTimeline, type TimelineEntry, type TimelineTone } from '@resources/components/EventTimeline';
import { formatDateOnly, formatTime, type LocaleFormat } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { cn } from '@resources/utils/cn';
import type { LifecycleEvent } from '../hooks/useLifecycle';
import { useLifecycleSubjectPages } from '../hooks/useLifecycleSubjectPages';
import {
  SEVERITY_LABEL,
  SUBJECT_ICON,
  buildSubjectPath,
  humanizeAction,
  type EnvironmentLite,
} from './lifecycle-format';

type LifecycleTimelineViewProps = {
  events: LifecycleEvent[];
  loading: boolean;
  formatPrefs: LocaleFormat;
  environments: EnvironmentLite[];
  onSelect: (event: LifecycleEvent) => void;
  emptyTitle: string;
};

export function LifecycleTimelineView({
  events,
  loading,
  formatPrefs,
  environments,
  onSelect,
  emptyTitle,
}: LifecycleTimelineViewProps) {
  const { t } = useTranslation('settings');
  const subjectPages = useLifecycleSubjectPages();

  const entries = useMemo<TimelineEntry[]>(
    () =>
      events.map((evt) => {
        const SubjectIcon = SUBJECT_ICON[evt.subjectType] ?? IconHistory;
        const style = severityStyle(evt.severity);
        const severityLabel = t(SEVERITY_LABEL[evt.severity] ?? 'lifecycle.severity.info');
        const envLabel =
          environments.find((e) => e.id === evt.environmentId)?.name ??
          evt.environmentId ??
          t('lifecycle.localEnvironment');

        return {
          id: evt.id,
          dateLabel: formatDateOnly(evt.timestamp, formatPrefs),
          time: formatTime(evt.timestamp, formatPrefs),
          timeTooltip: t('lifecycle.source', { source: evt.source }),
          tone: evt.severity as TimelineTone,
          icon: <SubjectIcon className="size-4" />,
          title: (
            <Link
              to={buildSubjectPath(evt, subjectPages)}
              className="hover:text-primary"
              title={evt.subjectName ?? evt.subjectId}
            >
              {evt.subjectName ?? evt.subjectId.slice(0, 12)}
            </Link>
          ),
          subtitle: humanizeAction(evt.action, t),
          badges: (
            <span
              className={cn(
                'inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-medium capitalize',
                style.pill,
                style.text,
              )}
            >
              <span className={cn('size-1.5 rounded-full', style.dot)} aria-hidden="true" />
              {severityLabel}
            </span>
          ),
          fields: [
            { label: t('lifecycle.headerSubject'), value: `${evt.subjectType} · ${envLabel}` },
            {
              label: t('lifecycle.headerDetail'),
              value: evt.state ? t(`lifecycle.state.${evt.state}`, { ns: 'common' }) : evt.eventType,
            },
          ],
          onSelect: () => onSelect(evt),
        };
      }),
    [environments, events, formatPrefs, onSelect, t, subjectPages],
  );

  return (
    <EventTimeline
      entries={entries}
      loading={loading}
      detailsLabel={t('lifecycle.viewDetails')}
      empty={{ icon: <IconHistory className="size-8" />, title: emptyTitle }}
    />
  );
}
