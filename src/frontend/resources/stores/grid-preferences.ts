// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type GridPreferences = {
  columnVisibility: Record<string, boolean>;
  columnOrder: string[];
  pageSize: number;
};

type GridPreferencesState = {
  preferences: Record<string, GridPreferences>;
  getPreferences: (gridId: string) => GridPreferences;
  setPreferences: (gridId: string, prefs: Partial<GridPreferences>) => void;
};

const defaultPreferences: GridPreferences = {
  columnVisibility: {},
  columnOrder: [],
  pageSize: 25,
};

export const useGridPreferencesStore = create<GridPreferencesState>()(
  persist(
    (set, get) => ({
      preferences: {},
      getPreferences: (gridId) => {
        return get().preferences[gridId] ?? defaultPreferences;
      },
      setPreferences: (gridId, prefs) => {
        set((state) => ({
          preferences: {
            ...state.preferences,
            [gridId]: {
              ...(state.preferences[gridId] ?? defaultPreferences),
              ...prefs,
            },
          },
        }));
      },
    }),
    { name: 'mcharbor-grid-preferences' }
  )
);
