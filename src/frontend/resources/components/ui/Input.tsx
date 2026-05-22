// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type InputHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

const inputVariants = cva(
  'w-full rounded-lg border text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary disabled:opacity-50 disabled:pointer-events-none',
  {
    variants: {
      variant: {
        default: 'py-2.5 px-4 bg-card border-border',
        outline: 'py-2 px-3 bg-background border-input focus:outline-none focus:ring-2 focus:ring-ring',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

type InputProps = InputHTMLAttributes<HTMLInputElement> & VariantProps<typeof inputVariants>;

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, variant, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(inputVariants({ variant, className }))}
      {...props}
    />
  )
);
Input.displayName = 'Input';

export { Input, inputVariants };
