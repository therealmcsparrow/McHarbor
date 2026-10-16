// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type HTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';
import { STATUS_COLORS } from '@resources/theme/status-palette';

// The status variants route through `status-palette.ts` rather than
// spelling their own class chains. That file is the single place a tone is
// defined, so a badge, a button and a status dot agree by construction
// instead of by three people remembering the same values.
export const badgeVariants = cva('inline-flex items-center gap-x-1.5 px-3 py-1.5 text-xs font-medium', {
  variants: {
    variant: {
      default: 'bg-primary/10 text-primary',
      secondary: 'bg-muted text-muted-foreground',
      neutral: 'bg-muted text-foreground',
      destructive: STATUS_COLORS.danger.soft,
      outline: 'border border-border text-foreground',
      success: STATUS_COLORS.success.soft,
      warning: STATUS_COLORS.warning.soft,
      info: STATUS_COLORS.info.soft,
    },
  },
  defaultVariants: {
    variant: 'default',
  },
});

type BadgeProps = HTMLAttributes<HTMLDivElement> & VariantProps<typeof badgeVariants>;

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}
