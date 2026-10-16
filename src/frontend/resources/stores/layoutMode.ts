// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type LayoutMode = 'auto' | 'oncall' | 'desktop';

type LayoutModeState = {
  mode: LayoutMode;
  setMode: (mode: LayoutMode) => void;
};

export const useLayoutModeStore = create<LayoutModeState>()(
  persist(
    (set) => ({
      mode: 'auto',
      setMode: (mode) => set({ mode }),
    }),
    { name: 'mcharbor-layout-mode', version: 1 },
  ),
);
