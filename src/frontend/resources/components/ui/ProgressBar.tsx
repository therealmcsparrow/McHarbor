// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { forwardRef, type HTMLAttributes } from 'react';
import { cn } from '@resources/utils/cn';

type ProgressBarSize = 'sm' | 'md' | 'lg';

const sizeClasses: Record<ProgressBarSize, string> = {
  sm: 'h-1.5',
  md: 'h-2.5',
  lg: 'h-4',
};

type ProgressBarProps = HTMLAttributes<HTMLDivElement> & {
  value: number;
  max?: number;
  size?: ProgressBarSize;
  showLabel?: boolean;
  labelClassName?: string;
};

const ProgressBar = forwardRef<HTMLDivElement, ProgressBarProps>(
  ({ className, value, max = 100, size = 'md', showLabel = false, labelClassName, ...props }, ref) => {
    const percentage = Math.min(Math.max((value / max) * 100, 0), 100);

    return (
      <div className={cn('w-full space-y-1.5', className)} {...props} ref={ref}>
        {showLabel && (
          <div className={cn('flex justify-between text-xs', labelClassName)}>
            <span className="text-muted-foreground">0%</span>
            <span className="text-muted-foreground">100%</span>
          </div>
        )}
        <div className={cn('w-full overflow-hidden rounded-full bg-muted', sizeClasses[size])}>
          <div
            className="h-full w-(--w) bg-primary transition-all duration-300 ease-out"
            style={cssVars({ w: `${percentage}%` })}
          />
        </div>
        {showLabel && <div className="text-center text-xs text-muted-foreground">{Math.round(percentage)}%</div>}
      </div>
    );
  },
);
ProgressBar.displayName = 'ProgressBar';

export { ProgressBar };
export type { ProgressBarProps, ProgressBarSize };
