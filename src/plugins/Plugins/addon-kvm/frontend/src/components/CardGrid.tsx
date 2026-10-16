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
export function CardGrid({
  count,
  isLoading,
  filtered,
  emptyLabel,
  children,
}: {
  count: number;
  isLoading: boolean;
  /** True when the list is empty only because the page's search matched nothing. */
  filtered: boolean;
  emptyLabel: string;
  children: ReactNode;
}) {
  const { t } = useTranslation('vm');

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <div key={`vm-card-skeleton-${index + 1}`} className="mc-glass min-h-[300px] animate-pulse rounded-xl p-4">
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
      <p className="py-12 text-center text-sm text-muted-foreground">{filtered ? t('list.noMatches') : emptyLabel}</p>
    );
  }

  return <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">{children}</div>;
}
