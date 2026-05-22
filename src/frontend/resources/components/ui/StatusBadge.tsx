// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { VariantProps } from 'class-variance-authority';
import { Badge, badgeVariants } from './Badge';
import { cn } from '@resources/utils/cn';

type BadgeVariant = NonNullable<VariantProps<typeof badgeVariants>['variant']>;

type StatusBadgeProps = {
  status: string;
  map?: Record<string, BadgeVariant>;
  className?: string;
};

export const CONTAINER_STATUS: Record<string, BadgeVariant> = {
  running: 'success',
  exited: 'destructive',
  stopped: 'destructive',
  created: 'secondary',
  restarting: 'warning',
  paused: 'warning',
  dead: 'destructive',
};

export const STACK_STATUS: Record<string, BadgeVariant> = {
  running: 'success',
  stopped: 'destructive',
  partial: 'warning',
};

export const ENVIRONMENT_STATUS: Record<string, BadgeVariant> = {
  connected: 'success',
  disconnected: 'destructive',
  error: 'destructive',
};

export const RECONCILER_STATUS: Record<string, BadgeVariant> = {
  synced: 'success',
  drifted: 'warning',
  pending: 'secondary',
  error: 'destructive',
};

export function StatusBadge({ status, map = {}, className }: StatusBadgeProps) {
  return (
    <Badge variant={map[status] ?? 'secondary'} className={cn(className)}>
      {status}
    </Badge>
  );
}
