// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type SavedSearch = {
  id: string;
  page: string;
  name: string;
  query: Record<string, string | number | boolean | undefined>;
  createdAt: number;
};

type SavedSearchesState = {
  saved: SavedSearch[];
  save: (page: string, name: string, query: Record<string, string | number | boolean | undefined>) => SavedSearch;
  remove: (id: string) => void;
  forPage: (page: string) => SavedSearch[];
};

const STORAGE_KEY = 'mcharbor-saved-searches-v1';

function makeId(): string {
  return `q_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

export const useSavedSearchesStore = create<SavedSearchesState>()(
  persist(
    (set, get) => ({
      saved: [],

      save: (page, name, query) => {
        const entry: SavedSearch = {
          id: makeId(),
          page,
          name,
          query,
          createdAt: Date.now(),
        };
        set((s) => ({ saved: [...s.saved, entry] }));
        return entry;
      },

      remove: (id) => {
        set((s) => ({ saved: s.saved.filter((q) => q.id !== id) }));
      },

      forPage: (page) => get().saved.filter((q) => q.page === page),
    }),
    {
      name: STORAGE_KEY,
      version: 1,
    },
  ),
);
