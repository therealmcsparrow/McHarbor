// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { cn } from '@resources/utils/cn';

type McSparrowDecorProps = {
  className?: string;
  size?: number;
  opacity?: number;
};

/**
 * Decorative McSparrow bird logo used as a visual accent in the dashboard.
 * This is a subtle background element that adds brand identity without
 * distracting from the main content.
 */
export function McSparrowDecor({ className, size = 120, opacity = 0.08 }: McSparrowDecorProps) {
  return (
    <img
      src="/logo_McSparrow.svg"
      alt=""
      role="presentation"
      aria-hidden="true"
      width={size}
      height={size}
      className={cn('pointer-events-none opacity-(--o) select-none', className)}
      style={cssVars({ o: opacity })}
    />
  );
}
