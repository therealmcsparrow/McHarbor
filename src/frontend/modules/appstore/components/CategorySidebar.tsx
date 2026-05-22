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
    <div className="w-full shrink-0 lg:min-h-0 lg:w-48 lg:overflow-y-auto lg:pr-1">
      <ul className="space-y-0.5">
        <li>
          <Button
            variant="ghost"
            onClick={() => onSelect('')}
            className={cn(
              'flex w-full items-center justify-between rounded-md px-3 py-1.5 text-sm transition-colors',
              selected === ''
                ? 'bg-primary/10 font-medium text-primary'
                : 'text-muted-foreground hover:bg-muted/50'
            )}
          >
            <span>{t('appStore.all')}</span>
            <span className="text-xs">{totalCount}</span>
          </Button>
        </li>
        {categories.map((cat) => (
          <li key={cat.category}>
            <Button
              variant="ghost"
              onClick={() => onSelect(cat.category)}
              className={cn(
                'flex w-full items-center justify-between rounded-md px-3 py-1.5 text-sm transition-colors',
                selected === cat.category
                  ? 'bg-primary/10 font-medium text-primary'
                  : 'text-muted-foreground hover:bg-muted/50'
              )}
            >
              <span>{labelMap?.[cat.category] ?? cat.category}</span>
              <span className="text-xs">{cat.count}</span>
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

