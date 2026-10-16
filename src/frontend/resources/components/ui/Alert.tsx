// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconInfoCircle, type TablerIcon } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

type AlertVariant = 'destructive' | 'warning' | 'info' | 'success';

const ALERT_VARIANT_CLASSES: Record<AlertVariant, string> = {
  destructive: 'border-destructive/40 bg-destructive/10 text-destructive',
  warning: 'border-warning/40 bg-warning/10 text-warning',
  info: 'border-info/40 bg-info/10 text-info',
  success: 'border-success/40 bg-success/10 text-success',
};

const ALERT_ICON_CLASSES: Record<AlertVariant, string> = {
  destructive: 'text-destructive',
  warning: 'text-warning',
  info: 'text-info',
  success: 'text-success',
};

const ALERT_DEFAULT_ICONS: Record<AlertVariant, TablerIcon> = {
  destructive: IconAlertCircle,
  warning: IconAlertTriangle,
  info: IconInfoCircle,
  success: IconCheck,
};

type AlertProps = {
  variant?: AlertVariant;
  title?: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  /** Hide the default variant icon. Useful when the message embeds its own icon. */
  hideIcon?: boolean;
  /** Optional action element (e.g. "Retry" button) rendered on the right. */
  action?: ReactNode;
  className?: string;
  children?: ReactNode;
};

/**
 * Alert is the canonical banner primitive for destructive / warning /
 * info / success callouts. Use it anywhere the inline ad-hoc
 * `rounded-md border border-destructive/40 bg-destructive/10` style
 * is currently being repeated.
 *
 * For status messages tied to a mutation, prefer `sonner` toasts.
 * Alerts are for context that lives inside the page (e.g. an
 * "unread changes" indicator, an outbound webhook failure notice).
 */
export function Alert({
  variant = 'info',
  title,
  description,
  icon,
  hideIcon = false,
  action,
  className,
  children,
}: AlertProps) {
  const Icon = ALERT_DEFAULT_ICONS[variant];
  const showIcon = !hideIcon;
  const renderedIcon =
    icon !== undefined ? icon : showIcon ? <Icon className="size-4 shrink-0" aria-hidden="true" /> : null;

  return (
    <div
      role={variant === 'destructive' || variant === 'warning' ? 'alert' : 'status'}
      className={cn('flex items-start gap-3 rounded-md border p-3 text-sm', ALERT_VARIANT_CLASSES[variant], className)}
    >
      {renderedIcon && <span className={cn('mt-0.5', ALERT_ICON_CLASSES[variant])}>{renderedIcon}</span>}
      <div className="min-w-0 flex-1 space-y-1">
        {title && <p className="font-medium text-foreground">{title}</p>}
        {description && <p className="text-xs leading-5 text-muted-foreground">{description}</p>}
        {children}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}
