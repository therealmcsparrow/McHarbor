// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';

type StoreItemCardProps = {
  id: string;
  icon: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  category: string;
  categoryClassName?: string;
  source: 'builtin' | 'downloaded' | 'plugin';
  checked: boolean;
  onToggle?: (id: string) => void;
  extra?: ReactNode;
  disabled?: boolean;
  disabledReason?: string;
};

const SOURCE_LABEL_KEYS: Record<StoreItemCardProps['source'], string> = {
  builtin: 'store.builtin',
  downloaded: 'store.downloaded',
  plugin: 'store.plugin',
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
        checked ? 'border-primary/50 bg-primary/5' : 'border-border bg-card hover:border-primary/30',
      )}
    >
      <div className="relative mt-0.5 shrink-0">
        <Switch
          checked={checked}
          disabled={disabled}
          onCheckedChange={() => {
            if (!disabled) {
              onToggle?.(id);
            }
          }}
          aria-label={title}
        />
      </div>

      {/* Icon */}
      <div className={cn('mt-0.5 shrink-0', checked ? 'text-primary' : 'text-muted-foreground')}>
        <Icon className="size-7" />
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{description}</p>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <Badge variant="outline" className={categoryClassName}>
            {category}
          </Badge>
          <Badge variant="secondary">{t(SOURCE_LABEL_KEYS[source])}</Badge>
          {extra}
        </div>
      </div>
    </label>
  );
}
