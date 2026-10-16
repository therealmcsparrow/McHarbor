// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The activity log's timeline rendering. Every column the table shows has a
// place here: timestamp (rail + date group), action (marker tone + badge),
// container (title + id field), image, exit code and environment. The eye
// button opens the same EventDetails dialog the table's actions column opens.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconActivity, IconBox } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { EventTimeline, type TimelineEntry, type TimelineTone } from '@resources/components/EventTimeline';
import { formatDate, formatDateOnly, formatTime, truncateId } from '@resources/utils/format';
import { ACTION_VARIANTS, parseMeta, type ContainerEvent } from './EventDetails';

// The table colours its action badge through ACTION_VARIANTS; the timeline
// marker needs the same signal expressed as a severity tone.
const TONE_BY_VARIANT: Record<string, TimelineTone> = {
  success: 'success',
  destructive: 'error',
  warning: 'warning',
  secondary: 'muted',
};

type ActivityTimelineViewProps = {
  events: ContainerEvent[];
  loading: boolean;
  envName: (id: string | null) => string;
  onSelect: (event: ContainerEvent) => void;
  emptyTitle: string;
};

export function ActivityTimelineView({ events, loading, envName, onSelect, emptyTitle }: ActivityTimelineViewProps) {
  const { t } = useTranslation('common');

  const entries = useMemo<TimelineEntry[]>(
    () =>
      events.map((evt) => {
        const meta = parseMeta(evt.metadata);
        const variant = ACTION_VARIANTS[evt.action] ?? 'secondary';
        const exitCode = meta.exitCode ?? null;

        return {
          id: evt.id,
          dateLabel: formatDateOnly(evt.timestamp),
          time: formatTime(evt.timestamp),
          timeTooltip: formatDate(evt.timestamp),
          tone: TONE_BY_VARIANT[variant] ?? 'muted',
          icon: <IconBox className="size-4" />,
          title: evt.containerName ?? truncateId(evt.containerId),
          subtitle: evt.eventType,
          badges: (
            <>
              <Badge variant={variant}>{evt.action}</Badge>
              {exitCode != null && (
                <Badge variant={exitCode === '0' ? 'secondary' : 'destructive'}>
                  {t('activity.columnExitCode')} {exitCode}
                </Badge>
              )}
            </>
          ),
          fields: [
            {
              label: t('activity.columnContainer'),
              value: truncateId(evt.containerId),
              mono: true,
            },
            { label: t('activity.columnEnvironment'), value: envName(evt.environmentId) },
            { label: t('activity.columnImage'), value: meta.image ?? '-', mono: true },
          ],
          onSelect: () => onSelect(evt),
        };
      }),
    [envName, events, onSelect, t],
  );

  return (
    <EventTimeline
      entries={entries}
      loading={loading}
      detailsLabel={t('activity.viewDetails')}
      empty={{ icon: <IconActivity className="size-8" />, title: emptyTitle }}
    />
  );
}
