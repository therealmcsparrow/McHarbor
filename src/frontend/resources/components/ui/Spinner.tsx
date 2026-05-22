// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

const spinnerVariants = cva(
  'animate-spin rounded-full border-primary border-t-transparent',
  {
    variants: {
      size: {
        sm: 'h-4 w-4 border-2',
        md: 'h-5 w-5 border-2',
        lg: 'h-6 w-6 border-2',
        xl: 'h-8 w-8 border-4',
      },
    },
    defaultVariants: {
      size: 'md',
    },
  }
);

type SpinnerProps = {
  className?: string;
} & VariantProps<typeof spinnerVariants>;

export function Spinner({ size, className }: SpinnerProps) {
  return <div className={cn(spinnerVariants({ size, className }))} />;
}
