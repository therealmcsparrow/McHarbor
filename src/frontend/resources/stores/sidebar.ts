// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type SidebarState = {
  /** Whole-sidebar icon-only collapse (orthogonal to per-section state). */
  collapsed: boolean;
  /** The one section currently unfolded, or null when all are folded. */
  expandedSection: string | null;
  toggle: () => void;
  toggleSection: (id: string) => void;
  setExpandedSection: (id: string | null) => void;
};

// Accordion by construction: a single id, so opening one section closes the
// rest without the caller having to hand the store the full section list.
// v1 persisted a `collapsedSections` array; the migration below drops it
// rather than trying to guess which of several open sections should win.
export const useSidebarStore = create<SidebarState>()(
  persist(
    (set) => ({
      collapsed: false,
      expandedSection: null,
      toggle: () => set((s) => ({ collapsed: !s.collapsed })),
      toggleSection: (id) => set((s) => ({ expandedSection: s.expandedSection === id ? null : id })),
      setExpandedSection: (id) => set({ expandedSection: id }),
    }),
    {
      name: 'mcharbor-sidebar',
      version: 2,
      migrate: (persisted) => {
        const prior = persisted as { collapsed?: boolean } | undefined;
        // Only the persisted slice is returned; zustand merges it over the
        // initial state, so the actions come back from the creator.
        return {
          collapsed: prior?.collapsed ?? false,
          expandedSection: null,
        } as SidebarState;
      },
    },
  ),
);
