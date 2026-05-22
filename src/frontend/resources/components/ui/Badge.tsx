// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type HTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

export const badgeVariants = cva(
  'inline-flex items-center gap-x-1.5 py-1.5 px-3 rounded-full text-xs font-medium',
  {
    variants: {
      variant: {
        default: 'bg-primary/10 text-primary',
        secondary: 'bg-muted text-muted-foreground',
        destructive: 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-400',
        outline: 'border border-border text-foreground',
        success: 'bg-teal-100 text-teal-800 dark:bg-teal-500/20 dark:text-teal-400',
        warning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-500/20 dark:text-yellow-400',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

type BadgeProps = HTMLAttributes<HTMLDivElement> & VariantProps<typeof badgeVariants>;

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}
