// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

type Options = {
  enabled: boolean;
  loading: boolean;
  onIntersect: () => void;
  rootMargin?: string;
};

export function useInfiniteScrollSentinel({
  enabled,
  loading,
  onIntersect,
  rootMargin = '200px',
}: Options) {
  const ref = useRef<HTMLDivElement | null>(null);
  const callbackRef = useRef(onIntersect);
  callbackRef.current = onIntersect;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (!entry?.isIntersecting) return;
        if (!enabled || loading) return;
        callbackRef.current();
      },
      { rootMargin },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [enabled, loading, rootMargin]);

  return ref;
}
