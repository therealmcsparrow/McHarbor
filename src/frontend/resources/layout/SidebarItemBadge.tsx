// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cn } from '@resources/utils/cn';

export type SidebarBadgeTone = 'default' | 'success' | 'warning' | 'destructive' | 'secondary';

export type SidebarItemBadgeData = {
  value: string;
  label: string;
  tone?: SidebarBadgeTone;
};

type SidebarItemBadgeProps = {
  badge: SidebarItemBadgeData;
  collapsed: boolean;
};

const TONE_CLASS: Record<SidebarBadgeTone, string> = {
  default: 'text-muted-foreground',
  secondary: 'text-muted-foreground',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-danger',
};

const TONE_PILL_CLASS: Record<SidebarBadgeTone, string> = {
  default: 'bg-primary text-primary-foreground',
  secondary: 'bg-muted text-muted-foreground',
  success: 'bg-success text-success-foreground',
  warning: 'bg-warning text-warning-foreground',
  destructive: 'bg-danger text-danger-foreground',
};

function capValue(value: string): string {
  const count = Number(value);
  return Number.isFinite(count) && count > 99 ? '99+' : value;
}

export function SidebarItemBadge({ badge, collapsed }: SidebarItemBadgeProps) {
  const tone = badge.tone ?? 'secondary';
  if (collapsed) {
    return (
      <span
        aria-label={badge.label}
        className={cn(
          'pointer-events-none absolute -top-0.5 -right-0.5 flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full px-1 text-[9px] leading-none font-semibold tabular-nums',
          TONE_PILL_CLASS[tone],
        )}
      >
        {capValue(badge.value)}
      </span>
    );
  }
  return (
    <span aria-label={badge.label} className={cn('shrink-0 text-[11px] tabular-nums', TONE_CLASS[tone])}>
      {badge.value}
    </span>
  );
}
