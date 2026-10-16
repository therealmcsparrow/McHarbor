// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createPortal } from 'react-dom';
import { useSyncExternalStore, type ReactNode } from 'react';
import { create } from 'zustand';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';

type SidebarSlotStore = {
  el: HTMLElement | null;
  filled: boolean;
  setEl: (el: HTMLElement | null) => void;
  setFilled: (filled: boolean) => void;
};

const useSidebarSlot = create<SidebarSlotStore>((set) => ({
  el: null,
  filled: false,
  setEl: (el) => set({ el }),
  setFilled: (filled) => set({ filled }),
}));

const WIDE_SCREEN = '(min-width: 2000px)';

function matchesWideScreen(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false;
  }
  return window.matchMedia(WIDE_SCREEN).matches;
}

function subscribeWideScreen(onChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return () => undefined;
  const query = window.matchMedia(WIDE_SCREEN);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

function useWideScreen(): boolean {
  return useSyncExternalStore(subscribeWideScreen, matchesWideScreen, () => false);
}

type PageSidebarProps = {
  className?: string;
};

export function PageSidebar({ className }: PageSidebarProps) {
  const { t } = useTranslation('common');
  const setEl = useSidebarSlot((store) => store.setEl);
  const filled = useSidebarSlot((store) => store.filled);
  const wide = useWideScreen();

  if (!wide) return null;

  return (
    <aside
      aria-label={t('pageSidebar.label')}
      className={cn('flex w-1/3 max-w-[34rem] min-w-[18rem] shrink-0 flex-col', className)}
    >
      <div className="flex min-h-0 flex-1 flex-col rounded-b-xl border border-t-0 border-border bg-card">
        <div
          data-scroll-exempt
          ref={(el) => {
            setEl(el);
            return () => setEl(null);
          }}
          className="flex min-h-0 flex-1 flex-col overflow-y-auto"
        />
        {!filled && <p className="px-4 py-6 text-center text-xs text-muted-foreground">{t('pageSidebar.empty')}</p>}
      </div>
    </aside>
  );
}

export function PageSidebarContent({ children }: { children: ReactNode }) {
  const el = useSidebarSlot((store) => store.el);
  const setFilled = useSidebarSlot((store) => store.setFilled);

  if (!el) return null;

  return createPortal(<SidebarFill onMount={setFilled}>{children}</SidebarFill>, el);
}

function SidebarFill({ children, onMount }: { children: ReactNode; onMount: (filled: boolean) => void }) {
  return (
    <div
      ref={(el) => {
        onMount(el !== null);
        return () => onMount(false);
      }}
      className="flex min-h-full flex-none flex-col"
    >
      {children}
    </div>
  );
}
