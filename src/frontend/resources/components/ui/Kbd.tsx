// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type HTMLAttributes } from 'react';
import { cn } from '@resources/utils/cn';

type KbdSize = 'sm' | 'md' | 'lg';

const sizeClasses: Record<KbdSize, string> = {
  sm: 'px-1.5 py-0.5 text-[10px]',
  md: 'px-2 py-1 text-xs',
  lg: 'px-2.5 py-1.5 text-sm',
};

type KbdProps = HTMLAttributes<HTMLSpanElement> & {
  size?: KbdSize;
  variant?: 'default' | 'muted' | 'primary' | 'destructive';
};

const variantClasses: Record<NonNullable<KbdProps['variant']>, string> = {
  default: 'bg-muted text-muted-foreground border border-border',
  muted: 'bg-muted/50 text-muted-foreground/80 border border-border/50',
  primary: 'bg-primary/10 text-primary border border-primary/20',
  destructive: 'bg-destructive/10 text-destructive border border-destructive/20',
};

const Kbd = forwardRef<HTMLSpanElement, KbdProps>(
  ({ className, size = 'md', variant = 'default', children, ...props }, ref) => (
    <span
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center rounded-md font-mono font-medium shadow-sm',
        sizeClasses[size],
        variantClasses[variant],
        className,
      )}
      {...props}
    >
      {children}
    </span>
  ),
);
Kbd.displayName = 'Kbd';

export { Kbd };
export type { KbdProps, KbdSize };
