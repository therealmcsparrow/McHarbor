// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// EventTimeline — the chronological counterpart to a log table.
//
// The activity log and the lifecycle log both page through timestamped events,
// so the timeline is written once here and fed by a per-page mapper. Every
// column the table shows has a home in an entry: the marker carries severity,
// the head line carries the primary subject and its badges, and `fields`
// carries the rest as label/value pairs — nothing is dropped in the switch.
//
// Deeper detail (raw metadata, ids, attribute dumps) stays behind the eye
// button, which opens the same dialog the table's actions column opens.

import { Fragment, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye } from '@tabler/icons-react';
import { Button } from './ui/Button';
import { EmptyState } from './ui/EmptyState';
import { Skeleton } from './ui/Skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/Tooltip';
import { severityStyle } from '@resources/utils/severity';
import { cn } from '@resources/utils/cn';

export type TimelineTone = 'info' | 'success' | 'warning' | 'error' | 'muted';

export type TimelineField = {
  label: string;
  value: ReactNode;
  /** Renders the value in the mono face — ids, images, digests. */
  mono?: boolean;
};

export type TimelineEntry = {
  id: string;
  /** Group heading — an already formatted calendar date. */
  dateLabel: string;
  /** Time of day, already formatted. */
  time: string;
  /** Optional tooltip on the time — full timestamp, event source. */
  timeTooltip?: string;
  tone: TimelineTone;
  icon: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  /** Sits beside the title — status, severity, exit code. */
  badges?: ReactNode;
  /** The remaining table columns, as label/value pairs. */
  fields?: TimelineField[];
  onSelect: () => void;
};

type EventTimelineProps = {
  entries: TimelineEntry[];
  loading?: boolean;
  empty?: { icon?: ReactNode; title: string; description?: string };
  /** Tooltip and aria-label for the per-entry details button. */
  detailsLabel: string;
};

function groupByDate(entries: TimelineEntry[]): { label: string; items: TimelineEntry[] }[] {
  const groups: { label: string; items: TimelineEntry[] }[] = [];
  for (const entry of entries) {
    const last = groups[groups.length - 1];
    if (last && last.label === entry.dateLabel) last.items.push(entry);
    else groups.push({ label: entry.dateLabel, items: [entry] });
  }
  return groups;
}

function TimelineRow({ entry, detailsLabel }: { entry: TimelineEntry; detailsLabel: string }) {
  const style = severityStyle(entry.tone === 'muted' ? '' : entry.tone);

  return (
    <li className="relative flex gap-3 pb-3">
      <div className="w-20 shrink-0 pt-3 text-right">
        {entry.timeTooltip ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-help font-mono text-xs text-muted-foreground">{entry.time}</span>
            </TooltipTrigger>
            <TooltipContent>{entry.timeTooltip}</TooltipContent>
          </Tooltip>
        ) : (
          <span className="font-mono text-xs text-muted-foreground">{entry.time}</span>
        )}
      </div>

      <div
        className={cn(
          'relative z-10 mt-2 flex size-8 shrink-0 items-center justify-center ring-4 ring-background',
          style.pill,
          style.text,
        )}
        aria-hidden="true"
      >
        {entry.icon}
      </div>

      <div className="min-w-0 flex-1 rounded-lg border border-border bg-card p-4">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="truncate text-sm font-medium text-foreground">{entry.title}</span>
              {entry.badges}
            </div>
            {entry.subtitle && <div className="mt-0.5 truncate text-xs text-muted-foreground">{entry.subtitle}</div>}
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="outline" size="icon-sm" onClick={entry.onSelect} aria-label={detailsLabel}>
                <IconEye className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{detailsLabel}</TooltipContent>
          </Tooltip>
        </div>

        {entry.fields && entry.fields.length > 0 && (
          <dl className="mt-2 flex flex-wrap gap-x-4 gap-y-1 border-t border-border pt-2 text-xs">
            {entry.fields.map((field) => (
              <Fragment key={field.label}>
                <dt className="text-muted-foreground">{field.label}</dt>
                <dd className={cn('max-w-64 truncate text-foreground', field.mono && 'font-mono')}>{field.value}</dd>
              </Fragment>
            ))}
          </dl>
        )}
      </div>
    </li>
  );
}

export function EventTimeline({ entries, loading, empty, detailsLabel }: EventTimelineProps) {
  const { t } = useTranslation('common');

  if (loading && entries.length === 0) {
    return (
      <div className="min-h-0 flex-1 space-y-3 overflow-auto" data-scroll-exempt>
        {Array.from({ length: 6 }).map((_, index) => (
          <div key={`timeline-skeleton-${index}`} className="flex gap-3">
            <Skeleton className="mt-3 h-3 w-20 shrink-0" />
            <Skeleton className="mt-2 size-8 shrink-0" rounded="full" />
            <Skeleton className="h-16 flex-1" rounded="lg" />
          </div>
        ))}
      </div>
    );
  }

  if (entries.length === 0) {
    return (
      <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
        <EmptyState icon={empty?.icon} title={empty?.title ?? t('dataGrid.noData')} description={empty?.description} />
      </div>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
      {groupByDate(entries).map((group) => (
        <section key={group.label} className="mb-2">
          <h3 className="sticky top-0 z-20 bg-background/95 py-1 pl-20 text-xs font-medium tracking-wider text-muted-foreground uppercase backdrop-blur">
            {group.label}
          </h3>
          <ol className="relative">
            <span
              aria-hidden="true"
              className="absolute top-3 bottom-3 left-[6.75rem] w-px -translate-x-1/2 bg-border"
            />
            {group.items.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} detailsLabel={detailsLabel} />
            ))}
          </ol>
        </section>
      ))}
    </div>
  );
}
