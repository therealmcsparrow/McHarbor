// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './Button';
import { cn } from '@resources/utils/cn';
import { STATUS_COLORS } from '@resources/theme/status-palette';

type EmptyStateAction = {
  label: string;
  onClick: () => void;
  variant?: 'default' | 'outline' | 'ghost';
};

type EmptyStateVariant = 'default' | 'warning' | 'destructive' | 'info' | 'compact';

type EmptyStateProps = {
  icon?: ReactNode;
  title: string;
  description?: string;
  primaryAction?: EmptyStateAction;
  secondaryAction?: EmptyStateAction;
  illustration?: ReactNode;
  className?: string;
  children?: ReactNode;
  /** Visual variant for the icon container + title accent. */
  variant?: EmptyStateVariant;
};

// Status variants route through the central palette rather than restating
// their own class chains — same reasoning as Badge and Button.
const variantClasses: Record<EmptyStateVariant, string> = {
  default: 'bg-muted text-muted-foreground',
  info: 'bg-primary/10 text-primary',
  warning: STATUS_COLORS.warning.soft,
  destructive: STATUS_COLORS.danger.soft,
  compact: 'bg-muted text-muted-foreground',
};

const variantSizing: Record<
  EmptyStateVariant,
  { wrapper: string; iconWrap: string; title: string; description: string }
> = {
  default: {
    wrapper: 'px-6 py-10',
    iconWrap: 'size-12',
    title: 'text-sm',
    description: 'text-sm',
  },
  info: {
    wrapper: 'px-6 py-10',
    iconWrap: 'size-12',
    title: 'text-sm',
    description: 'text-sm',
  },
  warning: {
    wrapper: 'px-6 py-10',
    iconWrap: 'size-12',
    title: 'text-sm',
    description: 'text-sm',
  },
  destructive: {
    wrapper: 'px-6 py-10',
    iconWrap: 'size-12',
    title: 'text-sm',
    description: 'text-sm',
  },
  compact: {
    wrapper: 'px-4 py-6',
    iconWrap: 'size-9',
    title: 'text-xs',
    description: 'text-xs',
  },
};

export function EmptyState({
  icon,
  title,
  description,
  primaryAction,
  secondaryAction,
  illustration,
  className,
  children,
  variant = 'default',
}: EmptyStateProps) {
  const sizing = variantSizing[variant];
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-lg border border-dashed border-border bg-muted/30 text-center',
        sizing.wrapper,
        className,
      )}
    >
      {illustration ??
        (icon && (
          <div className={cn('mb-3 flex items-center justify-center', sizing.iconWrap, variantClasses[variant])}>
            {icon}
          </div>
        ))}
      <h3 className={cn('font-semibold text-foreground', sizing.title)}>{title}</h3>
      {description && <p className={cn('mt-1 max-w-sm text-muted-foreground', sizing.description)}>{description}</p>}
      {(primaryAction || secondaryAction) && (
        <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
          {primaryAction && (
            <Button variant={primaryAction.variant ?? 'default'} onClick={primaryAction.onClick}>
              {primaryAction.label}
            </Button>
          )}
          {secondaryAction && (
            <Button variant={secondaryAction.variant ?? 'outline'} onClick={secondaryAction.onClick}>
              {secondaryAction.label}
            </Button>
          )}
        </div>
      )}
      {children}
    </div>
  );
}

type InlineEmptyProps = {
  icon?: ReactNode;
  message: string;
  className?: string;
};

export function InlineEmpty({ icon, message, className }: InlineEmptyProps) {
  return (
    <div className={cn('flex items-center justify-center gap-2 px-4 py-6 text-sm text-muted-foreground', className)}>
      {icon}
      <span>{message}</span>
    </div>
  );
}

type NoResultsProps = {
  searchTerm?: string;
  onClear?: () => void;
  className?: string;
};

export function NoResults({ searchTerm, onClear, className }: NoResultsProps) {
  const { t } = useTranslation('common');
  return (
    <EmptyState
      className={className}
      title={t('empty.noResults')}
      description={
        searchTerm
          ? t('empty.noResultsFor', {
              searchTerm,
            })
          : t('empty.tryDifferentFilters')
      }
      secondaryAction={
        onClear
          ? {
              label: t('actions.clear'),
              onClick: onClear,
              variant: 'outline',
            }
          : undefined
      }
    />
  );
}
