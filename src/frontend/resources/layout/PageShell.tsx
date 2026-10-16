// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// PageShell — the scroll contract, declared rather than sniffed.
//
// Whether a page scrolls itself or delegates to an inner region used to be
// decided by `isFlatCanvas` in AppLayout: a chain of nine
// `pathname.startsWith(...)` tests. Every new route had to remember to
// update a list in a file it had no other reason to open, and forgetting
// was silent — the page just scrolled wrong.
//
// The behaviour now lives with the route:
//
//   <PageShell scroll="data">   table / list pages — the inner region scrolls
//   <PageShell scroll="page">   detail / settings pages — flat canvas
//
// The contract this enforces:
//
//   AppLayout                    h-screen · overflow-hidden
//   ├── Sidebar                  fixed
//   └── main                     min-h-0 · flex-1
//       ├── Header               fixed
//       ├── PageShell            min-h-0 · flex-1
//       │   ├── PageToolbar      fixed
//       │   └── PageSplit        min-h-0 · flex-1 · row, optional
//       │       ├── PageBody     min-h-0 · flex-1 · THE ONLY SCROLLER
//       │       └── PageSidebar  w-1/3 · own scroller, data-scroll-exempt
//       └── StatusBar            fixed
//
//   1. One scroller per screen. Everything else is flex-none.
//   2. Tables scroll inside their own wrapper, never on the page body.
//   3. Wide content owns its overflow.
//   4. The page body never scrolls sideways, at any breakpoint.
//
// The sidebar is opt-in per route (`handle.sidebar`). The shell decides
// whether one exists; `PageSplit` decides where it starts, because a page's
// own header band usually has to sit above it rather than beside it.

import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react';
import { cn } from '@resources/utils/cn';
import { DockedBannerChips } from './DockableBanner';
import { PageSidebar } from './PageSidebar';

export type ScrollMode = 'data' | 'page';

const ScrollModeContext = createContext<ScrollMode>('data');
const SidebarEnabledContext = createContext(false);

export function usePageScrollMode(): ScrollMode {
  return useContext(ScrollModeContext);
}

/**
 * Dev-only guard: report subtrees that contain more than one scrollable
 * region. A second scroller is the single most common way this layout
 * breaks, and it is invisible until someone takes a screenshot at an
 * awkward height — so it surfaces at author time instead.
 */
function useSingleScrollerAssertion(ref: React.RefObject<HTMLDivElement | null>, mode: ScrollMode) {
  useEffect(() => {
    if (!import.meta.env.DEV) return;
    const root = ref.current;
    if (!root) return;

    // Let the subtree settle — data pages mount their grid asynchronously.
    const timer = window.setTimeout(() => {
      const scrollers: Element[] = [];
      const walk = (el: Element) => {
        const style = window.getComputedStyle(el);
        const scrolls = (v: string) => v === 'auto' || v === 'scroll';
        if (
          (scrolls(style.overflowY) && el.scrollHeight > el.clientHeight + 1) ||
          (scrolls(style.overflowX) && el.scrollWidth > el.clientWidth + 1)
        ) {
          // A table's own wrapper is the sanctioned exception — rule 2.
          if (!el.closest('[data-scroll-exempt]')) scrollers.push(el);
        }
        for (const child of Array.from(el.children)) walk(child);
      };
      walk(root);

      if (scrollers.length > 1) {
        console.warn(
          `[PageShell] ${scrollers.length} scrollable regions in one shell ` +
            `(scroll="${mode}"). Exactly one is expected — the PageBody. ` +
            'Mark a table/code/diagram wrapper with data-scroll-exempt if it ' +
            'legitimately owns its own overflow.',
          scrollers,
        );
      }
    }, 800);

    return () => window.clearTimeout(timer);
  }, [ref, mode]);
}

type PageShellProps = {
  /**
   * `data` — the page is a fixed frame and an inner region scrolls. Use for
   * tables, lists, grids: the toolbar stays put while rows move.
   *
   * `page` — flat canvas, the page itself scrolls. Use for detail and
   * settings pages, where content is prose-shaped and there is no row
   * region to keep anchored.
   */
  scroll: ScrollMode;
  /** Whether the shell owns the page gutter. Routes opt out with `handle.padded: false`. */
  padded?: boolean;
  /** Whether this route has a right sidebar. Routes opt in with `handle.sidebar: true`. */
  sidebar?: boolean;
  children: ReactNode;
  className?: string;
};

export function PageShell({ scroll: scrollMode, padded = true, sidebar = false, children, className }: PageShellProps) {
  const ref = useRef<HTMLDivElement>(null);
  useSingleScrollerAssertion(ref, scrollMode);

  return (
    <ScrollModeContext.Provider value={scrollMode}>
      <SidebarEnabledContext.Provider value={sidebar}>
        <div
          ref={ref}
          data-page-shell={scrollMode}
          className={cn(
            'flex h-full min-h-0 flex-1 flex-col',
            padded && 'p-6',
            // A data shell never scrolls itself; its PageBody does.
            scrollMode === 'data' ? 'overflow-hidden' : 'overflow-y-auto',
            className,
          )}
        >
          <DockedBannerChips className="px-4 pt-2" />
          {scrollMode === 'page' ? (
            // `min-h-full flex-none`, not `flex-1 min-h-0`: a flex-1 column is
            // pinned to the scroller's content-box height and lets the page
            // overflow *it* rather than the scroller's last in-flow box — and
            // overflow contributed by a descendant never picks up the scroll
            // container's `padding-bottom`, so the content ended flush against
            // the bottom edge. Growing to content height restores it, and
            // `min-h-full` still fills the viewport when the page is short.
            <div className="flex min-h-full w-full flex-none flex-col">{children}</div>
          ) : (
            children
          )}
        </div>
      </SidebarEnabledContext.Provider>
    </ScrollModeContext.Provider>
  );
}

/**
 * The row that holds the page body and, when the route opted in, the
 * sidebar. A route without `handle.sidebar` renders the row and nothing
 * else, so opting out costs nothing.
 */
export function PageSplit({ children, className }: { children: ReactNode; className?: string }) {
  const enabled = useContext(SidebarEnabledContext);
  return (
    <div className={cn('flex min-h-0 flex-1 p-6', className)}>
      {children}
      {enabled && <PageSidebar />}
    </div>
  );
}

/**
 * Wide content that owns its overflow — Compose YAML, inspect JSON, log
 * lines, diagrams, the workflow canvas, and any table wide enough to need
 * it. Rule 3: it scrolls sideways inside this box, never by pushing the
 * page body wider.
 */
export function PageOverflow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div data-scroll-exempt className={cn('max-w-full min-w-0 overflow-x-auto', className)}>
      {children}
    </div>
  );
}
