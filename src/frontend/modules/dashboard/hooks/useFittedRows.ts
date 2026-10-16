// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';

/**
 * How many `rowHeight`-pixel rows fit in the element the returned ref is
 * attached to. Re-measures on resize, so a widget resized in edit mode
 * shows more or fewer rows rather than growing a scrollbar.
 *
 * Lives here rather than beside the row primitives in
 * components/widget-kit-rows.tsx: it is measurement, not markup, and every
 * list widget uses it through the widget-kit barrel.
 */
export function useFittedRows(
  rowHeight: number,
  min = 1,
): { ref: (el: HTMLDivElement | null) => void; capacity: number } {
  const [capacity, setCapacity] = useState(min);
  const observerRef = useRef<ResizeObserver | null>(null);

  const ref = (el: HTMLDivElement | null) => {
    observerRef.current?.disconnect();
    observerRef.current = null;
    if (!el) return;
    const measure = () => {
      const next = Math.max(min, Math.floor(el.clientHeight / rowHeight));
      setCapacity((prev) => (prev === next ? prev : next));
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    observerRef.current = observer;
  };

  useEffect(
    () => () => {
      observerRef.current?.disconnect();
    },
    [],
  );

  return { ref, capacity };
}
