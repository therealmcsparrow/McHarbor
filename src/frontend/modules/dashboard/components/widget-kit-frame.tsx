// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Frame, header badge, empty/loading states and the footer link — the outer
// chrome every dashboard tile shares. See widget-kit.tsx for the overview.

import type { ReactNode } from 'react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import { TONE_SOFT, type Tone } from './widget-kit-tones';

type WidgetFrameProps = {
  title: string;
  /** Small text or a pill rendered right of the title. */
  badge?: ReactNode;
  /** A control rendered at the far right of the header. */
  action?: ReactNode;
  /** Optional strip under the body, e.g. a "view all" link. */
  footer?: ReactNode;
  /** Extra classes on the body column. */
  bodyClassName?: string;
  children: ReactNode;
};

export function WidgetFrame({ title, badge, action, footer, bodyClassName, children }: WidgetFrameProps) {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between gap-2 px-4 pt-3 pb-2">
        <div className="flex min-w-0 items-center gap-2">
          <h3 className="truncate text-sm font-semibold text-foreground">{title}</h3>
          {badge}
        </div>
        {action}
      </div>
      <div className={cn('flex min-h-0 flex-1 flex-col', bodyClassName)}>{children}</div>
      {footer && (
        <div className="flex shrink-0 items-center justify-between gap-2 border-t border-hairline px-4 py-1.5 text-xs text-muted-foreground">
          {footer}
        </div>
      )}
    </div>
  );
}

/** A small count pill for the header. */
export function WidgetBadge({ children, tone = 'neutral' }: { children: ReactNode; tone?: Tone }) {
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center gap-1 px-2 py-0.5 text-[11px] font-medium tabular-nums',
        TONE_SOFT[tone],
      )}
    >
      {children}
    </span>
  );
}

export function WidgetEmpty({
  icon,
  title,
  hint,
  tone = 'neutral',
  action,
}: {
  icon?: ReactNode;
  title: string;
  hint?: string;
  tone?: Tone;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-1.5 px-4 pb-3 text-center">
      {icon && <span className={cn('grid size-9 place-items-center rounded-lg', TONE_SOFT[tone])}>{icon}</span>}
      <p className="text-sm text-foreground">{title}</p>
      {hint && <p className="max-w-[28ch] text-xs text-muted-foreground">{hint}</p>}
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}

/** Centred spinner-less loading text, sized like the empty state. */
export function WidgetLoading({ label }: { label: string }) {
  return <div className="flex flex-1 items-center justify-center px-4 text-sm text-muted-foreground">{label}</div>;
}

/** A "+N more" / "view all" line for `WidgetFrame.footer`. */
export function WidgetFooterLink({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <PlainButton
      onClick={onClick}
      className="ml-auto text-xs text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {label} &rarr;
    </PlainButton>
  );
}
