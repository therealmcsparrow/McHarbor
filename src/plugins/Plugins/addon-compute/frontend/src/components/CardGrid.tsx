// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * The card view's frame: a responsive grid, skeletons and the empty line.
 *
 * It carries no search of its own — the page's filter bar sits above both views
 * and has already narrowed the rows, so a second box here would be a second
 * answer to the same question.
 */
const COLUMNS = {
  4: 'grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
  5: 'grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5',
} as const;

export function CardGrid({
  count,
  isLoading,
  filtered,
  emptyLabel,
  minHeight = 'min-h-[300px]',
  columns = 4,
  children,
}: {
  count: number;
  isLoading: boolean;
  /** True when the list is empty only because the page's search matched nothing. */
  filtered: boolean;
  emptyLabel: string;
  minHeight?: string;
  /** Widest column count, reached at 2xl. Below that every value grids the same. */
  columns?: 4 | 5;
  children: ReactNode;
}) {
  const { t } = useTranslation('compute');

  if (isLoading) {
    return (
      <div className={COLUMNS[columns]}>
        {Array.from({ length: columns }, (_, index) => (
          <div key={`card-skeleton-${index + 1}`} className={`${minHeight} mc-glass animate-pulse rounded-xl p-4`}>
            <div className="h-5 w-2/3 rounded bg-muted" />
            <div className="mt-2 h-4 w-1/2 rounded bg-muted" />
            <div className="mt-6 h-20 rounded-lg bg-muted" />
            <div className="mt-4 grid grid-cols-2 gap-2">
              <div className="h-16 rounded-lg bg-muted" />
              <div className="h-16 rounded-lg bg-muted" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (count === 0) {
    return (
      <p className="py-12 text-center text-sm text-muted-foreground">
        {filtered ? t('overview.noMatches') : emptyLabel}
      </p>
    );
  }

  return <div className={COLUMNS[columns]}>{children}</div>;
}
