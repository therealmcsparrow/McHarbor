// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { ListViewMode } from '@resources/components/ViewModeToggle';

type LifecycleViewState = {
  viewMode: ListViewMode;
  setViewMode: (mode: ListViewMode) => void;
};

export const useLifecycleViewStore = create<LifecycleViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-lifecycle-view' },
  ),
);
