// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type TextareaHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

// Surface, focus and hover come from `.mc-field` in app.css — the shared
// form-control material. Variants carry density only.
const textareaVariants = cva(
  'mc-field min-h-[80px] w-full rounded-lg border text-sm text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'px-4 py-2.5',
        outline: 'px-3 py-2',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & VariantProps<typeof textareaVariants>;

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(({ className, variant, ...props }, ref) => (
  <textarea ref={ref} className={cn(textareaVariants({ variant, className }))} {...props} />
));
Textarea.displayName = 'Textarea';

export { Textarea, textareaVariants };
