// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { IconX } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';

type TagTone = 'neutral' | 'primary' | 'muted' | 'success' | 'warning' | 'destructive' | 'info';

const toneClasses: Record<TagTone, string> = {
  neutral: 'bg-muted text-foreground/80 border-border',
  primary: 'bg-primary/10 text-primary border-primary/20',
  muted: 'bg-muted text-muted-foreground border-transparent',
  success: 'bg-success/15 text-success border-success/20',
  warning: 'bg-warning/15 text-warning border-warning/20',
  destructive: 'bg-destructive/15 text-destructive border-destructive/20',
  info: 'bg-info/15 text-info border-info/20',
};

const sizeClasses = {
  sm: 'h-5 px-1.5 text-[11px] gap-1',
  md: 'h-6 px-2 text-xs gap-1.5',
  lg: 'h-7 px-2.5 text-sm gap-1.5',
};

type TagSize = keyof typeof sizeClasses;

type TagProps = {
  tone?: TagTone;
  size?: TagSize;
  dismissible?: boolean;
  onDismiss?: () => void;
  dismissLabel?: string;
  icon?: ReactNode;
} & Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & {
    children: ReactNode;
  };

const Tag = forwardRef<HTMLButtonElement, TagProps>(
  (
    {
      className,
      tone = 'neutral',
      size = 'md',
      dismissible = false,
      onDismiss,
      dismissLabel = 'Remove',
      icon,
      children,
      type = 'button',
      ...props
    },
    ref,
  ) => {
    return (
      <PlainButton
        ref={ref}
        type={type}
        className={cn(
          'inline-flex items-center border font-medium transition-colors',
          sizeClasses[size],
          toneClasses[tone],
          'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:outline-none',
          'disabled:pointer-events-none disabled:opacity-50',
          className,
        )}
        {...props}
      >
        {icon ? (
          <span className="inline-flex shrink-0 items-center justify-center" aria-hidden="true">
            {icon}
          </span>
        ) : null}
        <span className="truncate">{children}</span>
        {dismissible ? (
          <PlainButton
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onDismiss?.();
            }}
            aria-label={dismissLabel}
            className="-mr-1 inline-flex size-4 shrink-0 items-center justify-center text-current/70 transition-colors hover:bg-foreground/10 hover:text-current focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
          >
            <IconX className="size-3" aria-hidden="true" />
          </PlainButton>
        ) : null}
      </PlainButton>
    );
  },
);
Tag.displayName = 'Tag';

export { Tag };
export type { TagProps, TagTone, TagSize };
