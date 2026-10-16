// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { CategoryCount } from '../types';

interface CategorySidebarProps {
  categories: CategoryCount[];
  selected: string;
  onSelect: (category: string) => void;
  totalCount: number;
  labelMap?: Record<string, string>;
}

export function CategorySidebar({ categories, selected, onSelect, totalCount, labelMap }: CategorySidebarProps) {
  const { t } = useTranslation('common');
  return (
    <div className="w-full shrink-0 overflow-x-auto pb-1 lg:min-h-0 lg:w-56 lg:overflow-x-hidden lg:overflow-y-auto lg:pr-1 lg:pb-0">
      <ul className="flex min-w-max gap-1 lg:block lg:min-w-0 lg:space-y-0.5">
        <li>
          <Button
            variant="ghost"
            onClick={() => onSelect('')}
            aria-pressed={selected === ''}
            className={cn(
              'flex w-auto items-center justify-between gap-3 rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring lg:w-full',
              selected === '' ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted/50',
            )}
          >
            <span className="min-w-0 truncate">{t('appStore.all')}</span>
            <span className="shrink-0 text-xs">{totalCount}</span>
          </Button>
        </li>
        {categories.map((cat) => (
          <li key={cat.category}>
            <Button
              variant="ghost"
              onClick={() => onSelect(cat.category)}
              aria-pressed={selected === cat.category}
              className={cn(
                'flex w-auto items-center justify-between gap-3 rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring lg:w-full',
                selected === cat.category
                  ? 'bg-primary/10 font-medium text-primary'
                  : 'text-muted-foreground hover:bg-muted/50',
              )}
            >
              <span className="min-w-0 truncate">{labelMap?.[cat.category] ?? cat.category}</span>
              <span className="shrink-0 text-xs">{cat.count}</span>
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
