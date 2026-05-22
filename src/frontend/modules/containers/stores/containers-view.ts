// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type ContainersViewState = {
  viewMode: 'table' | 'card';
  setViewMode: (mode: 'table' | 'card') => void;
};

export const useContainersViewStore = create<ContainersViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-containers-view' }
  )
);
