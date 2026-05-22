// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Badge } from '@resources/components/ui/Badge';

type StoreItemCardProps = {
  id: string;
  icon: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  category: string;
  categoryClassName?: string;
  source: 'builtin' | 'downloaded';
  checked: boolean;
  onToggle?: (id: string) => void;
  extra?: ReactNode;
  disabled?: boolean;
  disabledReason?: string;
};

export function StoreItemCard({
  id,
  icon: Icon,
  title,
  description,
  category,
  categoryClassName,
  source,
  checked,
  onToggle,
  extra,
  disabled = false,
  disabledReason,
}: StoreItemCardProps) {
  const { t } = useTranslation('common');

  return (
    <label
      aria-disabled={disabled}
      title={disabled ? disabledReason : undefined}
      className={cn(
        'flex gap-3 rounded-lg border p-3 transition-colors',
        disabled ? 'cursor-not-allowed opacity-55' : 'cursor-pointer',
        checked
          ? 'border-primary/50 bg-primary/5'
          : 'border-border bg-card hover:border-primary/30',
      )}
    >
      {/* Checkbox */}
      <div className="relative mt-0.5 shrink-0">
        <div
          className={cn(
            'flex size-4 items-center justify-center rounded border transition-colors',
            checked
              ? 'border-primary bg-primary text-primary-foreground'
              : 'border-muted-foreground/40 bg-background',
          )}
        >
          {checked && <IconCheck className="size-3" />}
        </div>
        <input
          type="checkbox"
          className="sr-only"
          checked={checked}
          disabled={disabled}
          onChange={() => {
            if (!disabled) {
              onToggle?.(id);
            }
          }}
          aria-label={title}
        />
      </div>

      {/* Icon */}
      <div className={cn('mt-0.5 shrink-0', checked ? 'text-primary' : 'text-muted-foreground')}>
        <Icon className="size-5" />
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="mt-0.5 text-xs text-muted-foreground line-clamp-2">{description}</p>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <Badge variant="outline" className={categoryClassName}>
            {category}
          </Badge>
          <Badge variant="secondary">
            {source === 'builtin' ? t('store.builtin') : t('store.downloaded')}
          </Badge>
          {extra}
        </div>
      </div>
    </label>
  );
}

