// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useLayoutEffect, useRef, useState, type ReactElement } from 'react';
import { ResponsiveContainer } from 'recharts';
import { cn } from '@resources/utils/cn';

type MeasuredResponsiveContainerProps = {
  children: ReactElement;
  className?: string;
  minHeight?: number;
  minWidth?: number;
};

export function MeasuredResponsiveContainer({
  children,
  className,
  minHeight = 1,
  minWidth = 1,
}: MeasuredResponsiveContainerProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [isReady, setIsReady] = useState(false);

  useLayoutEffect(() => {
    const element = rootRef.current;
    if (!element) return undefined;

    let frame = 0;

    const measure = () => {
      window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(() => {
        const { width, height } = element.getBoundingClientRect();
        setIsReady(width >= minWidth && height >= minHeight);
      });
    };

    measure();

    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => {
        window.cancelAnimationFrame(frame);
        window.removeEventListener('resize', measure);
      };
    }

    const observer = new ResizeObserver(measure);
    observer.observe(element);

    return () => {
      window.cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, [minHeight, minWidth]);

  return (
    <div ref={rootRef} className={cn('h-full min-h-0 w-full min-w-0', className)}>
      {isReady ? (
        <ResponsiveContainer width="100%" height="100%" minWidth={0} minHeight={0}>
          {children}
        </ResponsiveContainer>
      ) : null}
    </div>
  );
}
