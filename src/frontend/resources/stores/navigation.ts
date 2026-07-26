// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type RecentItem = {
  to: string;
  label: string;
  visitedAt: number;
};

type RecentsState = {
  recents: RecentItem[];
  favorites: string[];
  addRecent: (to: string, label: string) => void;
  toggleFavorite: (to: string) => void;
  isFavorite: (to: string) => boolean;
  clearRecents: () => void;
  clearFavorites: () => void;
};

const MAX_RECENTS = 10;
const STORAGE_KEY = 'mcharbor-navigation-v1';

export const useNavigationStore = create<RecentsState>()(
  persist(
    (set, get) => ({
      recents: [],
      favorites: [],

      addRecent: (to, label) => {
        set((s) => {
          const filtered = s.recents.filter((r) => r.to !== to);
          const next: RecentItem[] = [
            { to, label, visitedAt: Date.now() },
            ...filtered,
          ].slice(0, MAX_RECENTS);
          return { recents: next };
        });
      },

      toggleFavorite: (to) => {
        set((s) => {
          if (s.favorites.includes(to)) {
            return { favorites: s.favorites.filter((f) => f !== to) };
          }
          return { favorites: [...s.favorites, to] };
        });
      },

      isFavorite: (to) => get().favorites.includes(to),

      clearRecents: () => set({ recents: [] }),
      clearFavorites: () => set({ favorites: [] }),
    }),
    {
      name: STORAGE_KEY,
      version: 1,
    },
  ),
);
