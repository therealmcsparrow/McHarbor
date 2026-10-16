// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type RefObject } from 'react';

// Which section the reader is actually looking at.
//
// The viewer's rail is a SubNav, and a SubNav's pill is a claim about where
// you are — not about where you last clicked. Without this the pill would go
// stale the moment someone scrolled, which is the normal way to read a
// report. Observed against the viewer's own scroll container rather than the
// window, because the document scrolls inside the page shell, not with it.

export function useSectionSpy(scrollRef: RefObject<HTMLElement | null>, ids: readonly string[]): string {
  const [active, setActive] = useState<string>(ids[0] ?? '');

  useEffect(() => {
    const root = scrollRef.current;
    if (!root || ids.length === 0) return;

    const visible = new Set<string>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const id = entry.target.id;
          if (entry.isIntersecting) visible.add(id);
          else visible.delete(id);
        }
        // Topmost visible section wins — the one whose heading the reader
        // has most recently passed.
        const next = ids.find((id) => visible.has(id));
        if (next) setActive(next);
      },
      // A band across the upper third: a section counts as "current" once
      // its heading clears the hero, and stops counting well before it
      // leaves the viewport.
      { root, rootMargin: '-8% 0px -70% 0px', threshold: 0 },
    );

    for (const id of ids) {
      const el = root.querySelector(`#${CSS.escape(id)}`);
      if (el) observer.observe(el);
    }

    return () => observer.disconnect();
  }, [scrollRef, ids]);

  return ids.includes(active) ? active : (ids[0] ?? '');
}
