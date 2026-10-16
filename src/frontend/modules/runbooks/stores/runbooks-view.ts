// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type RunbooksViewState = {
  viewMode: 'table' | 'card';
  setViewMode: (mode: 'table' | 'card') => void;
};

export const useRunbooksViewStore = create<RunbooksViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-runbooks-view' },
  ),
);
