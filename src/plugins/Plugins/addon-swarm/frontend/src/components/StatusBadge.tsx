// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cn } from '@resources/utils/cn';
import { statusSoft, type StatusTone } from '@resources/theme/status-palette';

type StatusBadgeProps = {
  tone: StatusTone;
  children: React.ReactNode;
  className?: string;
};

/**
 * A tone-token badge.
 *
 * The shared `<Badge>` offers four fixed variants (success, warning,
 * destructive, secondary), which cannot express the six states SwarmKit
 * actually has — `complete` and `orphaned` both collapse into "not success"
 * there. This routes straight through the same `status-palette` tokens the
 * Badge itself uses, so a Swarm state badge and a container state badge agree
 * by construction rather than by coincidence.
 */
export function StatusBadge({ tone, children, className }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-x-1.5 rounded-full px-2 py-0.5 text-xs font-medium',
        statusSoft(tone),
        className,
      )}
    >
      {children}
    </span>
  );
}
