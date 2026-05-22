// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type StacksViewState = {
  viewMode: 'table' | 'card';
  setViewMode: (mode: 'table' | 'card') => void;
};

export const useStacksViewStore = create<StacksViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-stacks-view' }
  )
);
