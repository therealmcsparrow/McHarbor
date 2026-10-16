// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StatusIndicator — one status presentation, everywhere.
//
// The same running container used to be drawn four different ways: a teal
// dot on the card, a green badge in the table row, an emerald pill in the
// detail header, and something else again in each widget. None of them was
// wrong on its own; together they meant the user had to learn the mapping
// separately in each place, and a colour change had to be made in four.
//
// Everything now routes through here, fed by `containerStatePresentation`
// and `status-palette.ts`. Same dot, same label, same tone.
//
// The running state — and only the running state — pulses, because it is
// the only one that is continuously true rather than a fact about the past.
// The pulse is gated on `prefers-reduced-motion` and the in-app motion
// preference, so it holds still for anyone who asked for that.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';
import { STATUS_COLORS, type StatusTone } from '@resources/theme/status-palette';

export type StatusIndicatorVariant = 'dot' | 'badge' | 'pill';

export type StatusIndicatorSize = 'sm' | 'md';

type StatusIndicatorProps = {
  tone: StatusTone;
  /** Visible label. Already translated. Omit for a bare dot. */
  label?: ReactNode;
  variant?: StatusIndicatorVariant;
  size?: StatusIndicatorSize;
  /**
   * Animate the dot. Reserved for continuously-true states — a running
   * container — never for a state that merely happens to be current.
   */
  pulse?: boolean;
  /**
   * Accessible name when there is no visible label. Required for a bare
   * dot: a coloured circle with no text is invisible to a screen reader.
   */
  ariaLabel?: string;
  className?: string;
};

const DOT_SIZE: Record<StatusIndicatorSize, string> = {
  sm: 'size-1.5',
  md: 'size-2',
};

const TEXT_SIZE: Record<StatusIndicatorSize, string> = {
  sm: 'text-[10px]',
  md: 'text-xs',
};

export function StatusIndicator({
  tone,
  label,
  variant = 'badge',
  size = 'md',
  pulse = false,
  ariaLabel,
  className,
}: StatusIndicatorProps) {
  const token = STATUS_COLORS[tone];

  const dot = (
    <span
      aria-hidden={label ? 'true' : undefined}
      role={label ? undefined : 'status'}
      aria-label={label ? undefined : ariaLabel}
      className={cn('shrink-0 rounded-full', DOT_SIZE[size], token.dot, pulse && 'mc-status-pulse')}
    />
  );

  if (variant === 'dot' || !label) {
    return <span className={cn('inline-flex', className)}>{dot}</span>;
  }

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 font-medium',
        TEXT_SIZE[size],
        variant === 'badge' && cn('px-2 py-0.5', token.soft),
        variant === 'pill' && cn('px-2 py-0.5', token.outline),
        className,
      )}
    >
      {dot}
      {label}
    </span>
  );
}
