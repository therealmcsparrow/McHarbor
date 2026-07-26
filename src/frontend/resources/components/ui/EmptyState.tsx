// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './Button';
import { cn } from '@resources/utils/cn';

type EmptyStateAction = {
  label: string;
  onClick: () => void;
  variant?: 'default' | 'outline' | 'ghost';
};

type EmptyStateProps = {
  icon?: ReactNode;
  title: string;
  description?: string;
  primaryAction?: EmptyStateAction;
  secondaryAction?: EmptyStateAction;
  illustration?: ReactNode;
  className?: string;
  children?: ReactNode;
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
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-lg border border-dashed border-border bg-muted/30 px-6 py-10 text-center',
        className,
      )}
    >
      {illustration ?? (
        icon && (
          <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            {icon}
          </div>
        )
      )}
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      {description && (
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      )}
      {(primaryAction || secondaryAction) && (
        <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
          {primaryAction && (
            <Button
              variant={primaryAction.variant ?? 'default'}
              onClick={primaryAction.onClick}
            >
              {primaryAction.label}
            </Button>
          )}
          {secondaryAction && (
            <Button
              variant={secondaryAction.variant ?? 'outline'}
              onClick={secondaryAction.onClick}
            >
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
    <div
      className={cn(
        'flex items-center justify-center gap-2 px-4 py-6 text-sm text-muted-foreground',
        className,
      )}
    >
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
      title={t('empty.noResults', { defaultValue: 'No results found' })}
      description={
        searchTerm
          ? t('empty.noResultsFor', {
              searchTerm,
              defaultValue: `No results match "${searchTerm}".`,
            })
          : t('empty.tryDifferentFilters', {
              defaultValue: 'Try adjusting your filters or search term.',
            })
      }
      secondaryAction={
        onClear
          ? {
              label: t('actions.clear', { defaultValue: 'Clear' }),
              onClick: onClear,
              variant: 'outline',
            }
          : undefined
      }
    />
  );
}
