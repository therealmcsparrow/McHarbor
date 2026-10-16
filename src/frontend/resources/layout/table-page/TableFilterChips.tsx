// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { ActiveFilterChip } from './types';

type TableFilterChipsProps = {
  items: ActiveFilterChip[];
  /** Rendered as the `filters.matchCount` line. Omit to hide it. */
  matchCount?: number;
};

export function TableFilterChips({ items, matchCount }: TableFilterChipsProps) {
  const { t } = useTranslation('common');

  if (items.length === 0 && matchCount === undefined) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2" aria-label={t('table.activeFilters')}>
      {items.map((chip) => (
        <span
          key={chip.id}
          className="inline-flex items-center gap-1 border border-border bg-muted/50 py-0.5 pr-1 pl-2.5 text-xs text-foreground"
        >
          {chip.label}
          <Button
            variant="ghost"
            size="icon-sm"
            className="size-5"
            aria-label={t('table.removeFilter')}
            onClick={chip.onRemove}
          >
            <IconX className="size-3" />
          </Button>
        </span>
      ))}
      {matchCount !== undefined && (
        <span className="text-xs text-muted-foreground">{t('filters.matchCount', { count: matchCount })}</span>
      )}
    </div>
  );
}
