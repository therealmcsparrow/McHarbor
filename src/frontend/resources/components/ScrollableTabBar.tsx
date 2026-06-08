// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';

type ScrollableTabBarProps = {
  children: ReactNode;
  className?: string;
  listClassName?: string;
  fadeClassName?: string;
};

export function ScrollableTabBar({
  children,
  className,
  listClassName,
  fadeClassName,
}: ScrollableTabBarProps) {
  const { t } = useTranslation('common');
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const updateScrollState = useCallback(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;

    const maxScrollLeft = viewport.scrollWidth - viewport.clientWidth;
    setCanScrollLeft(viewport.scrollLeft > 1);
    setCanScrollRight(viewport.scrollLeft < maxScrollLeft - 1);
  }, []);

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;

    updateScrollState();
    const resizeObserver = new ResizeObserver(updateScrollState);
    resizeObserver.observe(viewport);

    viewport.addEventListener('scroll', updateScrollState, { passive: true });
    window.addEventListener('resize', updateScrollState);
    return () => {
      resizeObserver.disconnect();
      viewport.removeEventListener('scroll', updateScrollState);
      window.removeEventListener('resize', updateScrollState);
    };
  }, [updateScrollState]);

  useEffect(() => {
    updateScrollState();
  }, [children, updateScrollState]);

  const scrollByPage = (direction: -1 | 1) => {
    const viewport = viewportRef.current;
    if (!viewport) return;

    viewport.scrollBy({
      left: direction * Math.max(160, viewport.clientWidth * 0.7),
      behavior: 'smooth',
    });
  };

  const hasOverflow = canScrollLeft || canScrollRight;

  return (
    <div className={cn('relative min-w-0', className)}>
      {hasOverflow && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('dataGrid.prev')}
          disabled={!canScrollLeft}
          onClick={() => scrollByPage(-1)}
          className="absolute left-0 top-1/2 z-20 -translate-y-1/2 bg-card/95 shadow-sm"
        >
          <IconChevronLeft className="size-4" />
        </Button>
      )}

      {canScrollLeft && (
        <div
          aria-hidden="true"
          className={cn('pointer-events-none absolute left-8 top-0 z-10 h-full w-8 bg-gradient-to-r from-card to-transparent', fadeClassName)}
        />
      )}

      <div
        ref={viewportRef}
        className={cn(
          'min-w-0 overflow-x-auto scroll-smooth [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
          hasOverflow && 'px-8',
        )}
      >
        <div className={cn('flex w-max gap-x-1 rounded-lg bg-muted p-1', listClassName)}>
          {children}
        </div>
      </div>

      {canScrollRight && (
        <div
          aria-hidden="true"
          className={cn('pointer-events-none absolute right-8 top-0 z-10 h-full w-8 bg-gradient-to-l from-card to-transparent', fadeClassName)}
        />
      )}

      {hasOverflow && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('dataGrid.next')}
          disabled={!canScrollRight}
          onClick={() => scrollByPage(1)}
          className="absolute right-0 top-1/2 z-20 -translate-y-1/2 bg-card/95 shadow-sm"
        >
          <IconChevronRight className="size-4" />
        </Button>
      )}
    </div>
  );
}
