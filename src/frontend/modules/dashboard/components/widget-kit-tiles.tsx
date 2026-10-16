// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Stat tile, segment bar and the compact relative-time formatter. See
// widget-kit.tsx for the overview.

import type { ReactNode } from 'react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';
import { ToneDot } from './widget-kit-rows';
import { TONE_SOLID, TONE_TEXT, type Tone } from './widget-kit-tones';

type WidgetTileProps = {
  label: string;
  value: ReactNode;
  /** Small line under the value. */
  hint?: ReactNode;
  tone?: Tone;
  icon?: ReactNode;
  /** 0..1 — draws a thin proportion bar along the bottom. */
  share?: number;
  onClick?: () => void;
  className?: string;
};

/**
 * A stat cell. Put several in a `grid` with `flex-1 min-h-0` and each fills
 * its track top-to-bottom, so a 2×2 of them uses the whole tile.
 */
export function WidgetTile({ label, value, hint, tone = 'neutral', icon, share, onClick, className }: WidgetTileProps) {
  const inner = (
    <>
      <span className="flex items-center justify-between gap-2">
        <span className="truncate text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{label}</span>
        {icon && <span className={cn('shrink-0', TONE_TEXT[tone])}>{icon}</span>}
      </span>
      <span className="flex min-h-0 flex-1 flex-col justify-center">
        <span
          className={cn(
            'text-2xl leading-none font-semibold tabular-nums',
            tone === 'neutral' ? 'text-foreground' : TONE_TEXT[tone],
          )}
        >
          {value}
        </span>
        {hint && <span className="mt-1 truncate text-[11px] text-muted-foreground">{hint}</span>}
      </span>
      {typeof share === 'number' && (
        <span className="h-1 w-full overflow-hidden rounded-full bg-glass-2" aria-hidden="true">
          <span
            className={cn('block h-full w-(--w) rounded-full', TONE_SOLID[tone === 'neutral' ? 'primary' : tone])}
            // The fill is the data; it travels as a CSS variable the class reads.
            style={cssVars({ w: `${Math.round(Math.max(0, Math.min(1, share)) * 100)}%` })}
          />
        </span>
      )}
    </>
  );
  const classes = cn(
    'flex min-h-0 flex-col gap-1.5 rounded-xl border border-hairline bg-glass px-3 py-2.5 text-left',
    onClick &&
      'transition-colors hover:border-hairline-2 hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
    className,
  );
  if (onClick) {
    return (
      <PlainButton onClick={onClick} className={classes}>
        {inner}
      </PlainButton>
    );
  }
  return <div className={classes}>{inner}</div>;
}

export type Segment = {
  key: string;
  label: string;
  count: number;
  tone: Tone;
  onClick?: () => void;
};

/**
 * One proportional bar with the counts written on the segments, plus a
 * legend row. Segments with a zero count are skipped.
 */
function segmentTag(clickable: boolean) {
  return clickable ? 'button' : 'span';
}

export function SegmentBar({
  segments,
  total,
  className,
}: {
  segments: Segment[];
  total?: number;
  className?: string;
}) {
  const visible = segments.filter((s) => s.count > 0);
  const sum = total ?? visible.reduce((acc, s) => acc + s.count, 0);
  if (visible.length === 0 || sum === 0) return null;
  return (
    <div className={cn('shrink-0', className)}>
      <div className="flex h-7 w-full gap-0.5">
        {visible.map((s) => {
          const Comp = segmentTag(Boolean(s.onClick));
          return (
            <Comp
              key={s.key}
              type={s.onClick ? 'button' : undefined}
              onClick={s.onClick}
              aria-label={`${s.count} ${s.label}`}
              className={cn(
                'flex min-w-7 [flex-grow:var(--grow)] basis-0 items-center justify-center rounded-sm text-xs font-semibold text-background tabular-nums first:rounded-l-lg last:rounded-r-lg',
                TONE_SOLID[s.tone],
                s.onClick && 'transition-opacity hover:opacity-85',
              )}
              // Flex growth is the data; it travels as a CSS variable the class reads.
              style={cssVars({ grow: s.count })}
            >
              {s.count}
            </Comp>
          );
        })}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-0.5">
        {visible.map((s) => (
          <span key={s.key} className="inline-flex items-center gap-1.5 text-[11px] text-muted-foreground">
            <ToneDot tone={s.tone} />
            {s.label}
          </span>
        ))}
      </div>
    </div>
  );
}

/**
 * Compact relative time for a narrow column: "just now", "45s ago",
 * "5m ago", "3h ago", "2d ago". The host's `timeAgo` says "now" for
 * anything under a minute, which is useless the moment a restart drops
 * six events into the feed within the same second.
 */
export function relativeTime(iso: string | undefined | null, justNow: string, locale?: string): string {
  if (!iso) return '-';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return '-';
  const diff = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (diff < 5) return justNow;
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'always', style: 'narrow' });
  if (diff < 60) return rtf.format(-diff, 'second');
  if (diff < 3600) return rtf.format(-Math.floor(diff / 60), 'minute');
  if (diff < 86400) return rtf.format(-Math.floor(diff / 3600), 'hour');
  return rtf.format(-Math.floor(diff / 86400), 'day');
}
