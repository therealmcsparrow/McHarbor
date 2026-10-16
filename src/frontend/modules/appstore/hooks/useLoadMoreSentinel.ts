// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

// Calls onLoadMore when the returned ref's element nears the viewport of its
// scroll container. The margin starts the next page before the user hits the
// end, so a long catalog scrolls without a visible stall. `loaded` re-arms the
// observer after each page: a sentinel still in view never fires a second time.
export function useLoadMoreSentinel<T extends HTMLElement>(enabled: boolean, loaded: number, onLoadMore: () => void) {
  const ref = useRef<T>(null);
  const callback = useRef(onLoadMore);

  useEffect(() => {
    callback.current = onLoadMore;
  });

  useEffect(() => {
    const node = ref.current;
    if (!enabled || !node || typeof IntersectionObserver === 'undefined') return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) callback.current();
      },
      { rootMargin: '600px 0px' },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [enabled, loaded]);

  return ref;
}
