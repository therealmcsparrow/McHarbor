// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createPortal } from 'react-dom';
import type { ReactNode } from 'react';
import { create } from 'zustand';
import { useHeaderSlot } from '@resources/hooks/useHeaderSlot';
import { cn } from '@resources/utils/cn';

type ActionsSlotStore = {
  el: HTMLElement | null;
  setEl: (el: HTMLElement | null) => void;
};

// Not a `getElementById` on a well-known id, which is how the topbar slots
// work: this element is itself inside a portal, so it reaches the DOM a render
// later than the component that wants to fill it, and a one-shot lookup misses
// it every time. A ref callback publishes the node on commit instead, and the
// subscriber re-renders when it arrives.
const useActionsSlot = create<ActionsSlotStore>((set) => ({
  el: null,
  setEl: (el) => set({ el }),
}));

type PageHeadIdentityProps = {
  children: ReactNode;
  className?: string;
};

export function PageHeadIdentity({ children, className }: PageHeadIdentityProps) {
  const slot = useHeaderSlot('pagehead-slot');
  const setActionsEl = useActionsSlot((store) => store.setEl);
  if (!slot) return null;

  return createPortal(
    <div
      className={cn(
        'flex h-20 min-w-0 shrink-0 items-center gap-3 overflow-hidden border-b border-border bg-canvas-2 px-6',
        className,
      )}
    >
      {children}
      <div
        ref={(el) => {
          setActionsEl(el);
          return () => setActionsEl(null);
        }}
        className="ml-auto flex shrink-0 items-center justify-end"
      />
    </div>,
    slot,
  );
}

export function PageHeadActions({ children }: { children: ReactNode }) {
  const el = useActionsSlot((store) => store.el);
  if (!el) return null;

  return createPortal(children, el);
}
