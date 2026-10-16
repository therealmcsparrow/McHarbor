// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Fitted-row measurement and the list row primitives. See widget-kit.tsx
// for the overview.

import type { ReactNode } from 'react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import { TONE_SOFT, TONE_SOLID, type Tone } from './widget-kit-tones';

/** Standard row height the list widgets share, in px. */
export const ROW_HEIGHT = 40;

type WidgetRowProps = {
  /** Icon, dot or avatar at the left edge. */
  leading?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  /** Right-aligned content: a badge, a number, a time. */
  trailing?: ReactNode;
  onClick?: () => void;
  className?: string;
};

export function WidgetRow({ leading, title, subtitle, trailing, onClick, className }: WidgetRowProps) {
  const body = (
    <>
      {leading && <span className="flex shrink-0 items-center">{leading}</span>}
      <span className="flex min-w-0 flex-1 flex-col justify-center">
        <span className="truncate text-sm leading-tight text-foreground">{title}</span>
        {subtitle && <span className="truncate text-[11px] leading-tight text-muted-foreground">{subtitle}</span>}
      </span>
      {trailing && <span className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">{trailing}</span>}
    </>
  );
  const classes = cn(
    'flex h-10 w-full items-center gap-2.5 rounded-md px-2 text-left',
    onClick &&
      'transition-colors hover:bg-glass-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
    className,
  );
  if (onClick) {
    return (
      <PlainButton onClick={onClick} className={classes}>
        {body}
      </PlainButton>
    );
  }
  return <div className={classes}>{body}</div>;
}

/** The list container `WidgetRow`s live in. Attach the fitted-rows ref here. */
export function WidgetList({
  listRef,
  children,
}: {
  listRef?: (el: HTMLDivElement | null) => void;
  children: ReactNode;
}) {
  return (
    <div ref={listRef} className="min-h-0 flex-1 overflow-hidden px-2">
      <ul className="flex flex-col">{children}</ul>
    </div>
  );
}

/** A coloured dot for row leading slots. */
export function ToneDot({ tone, className }: { tone: Tone; className?: string }) {
  return <span aria-hidden="true" className={cn('size-2 shrink-0 rounded-full', TONE_SOLID[tone], className)} />;
}

/** A square tinted tile holding an icon, for row leading slots. */
export function IconTile({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={cn('grid size-7 shrink-0 place-items-center rounded-md', TONE_SOFT[tone])}>{children}</span>;
}
