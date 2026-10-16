// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Docked banners — where a dismissed banner goes.
//
// Banners used to stack above the data: on a 900px viewport in a degraded
// state, roughly 40% of the screen was chrome before row one. The fix is
// not to delete them — a degraded connection is exactly the thing a user
// needs told — but to let them collapse to a chip and be reopened.
//
// Dismissing files the banner here; `PageToolbar` renders one compact chip
// per docked banner, so nothing becomes unreachable. The store persists,
// because a banner dismissed on Tuesday should not reappear on Wednesday
// just because the tab reloaded.
//
// Note: the plan called for dismissed banners to land in the notification
// centre. That surface is server-generated (`/in-app-notifications` has no
// create endpoint), so filing into it would need a backend contract that
// is out of scope here. The toolbar chip provides the same guarantee —
// reachable, reopenable, never silently lost.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type DockedBanner = {
  /** Stable identity — same banner across reloads. */
  id: string;
  /** Short label for the chip. Already translated by the caller. */
  label: string;
  tone: 'info' | 'warning' | 'danger';
  dockedAt: number;
};

type DockedBannersState = {
  docked: Record<string, DockedBanner>;
  dock: (banner: Omit<DockedBanner, 'dockedAt'>) => void;
  restore: (id: string) => void;
  isDocked: (id: string) => boolean;
  list: () => DockedBanner[];
  clear: () => void;
};

export const useDockedBanners = create<DockedBannersState>()(
  persist(
    (set, get) => ({
      docked: {},

      dock: (banner) =>
        set((state) => ({
          docked: {
            ...state.docked,
            [banner.id]: { ...banner, dockedAt: Date.now() },
          },
        })),

      restore: (id) =>
        set((state) => {
          if (!(id in state.docked)) return state;
          const next = { ...state.docked };
          delete next[id];
          return { docked: next };
        }),

      isDocked: (id) => id in get().docked,

      list: () => Object.values(get().docked).sort((a, b) => a.dockedAt - b.dockedAt),

      clear: () => set({ docked: {} }),
    }),
    { name: 'mcharbor-docked-banners' },
  ),
);
