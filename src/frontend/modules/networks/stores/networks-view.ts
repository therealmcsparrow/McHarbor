// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type NetworksViewMode = 'table' | 'card';

type NetworksViewState = {
  viewMode: NetworksViewMode;
  setViewMode: (mode: NetworksViewMode) => void;
};

export const useNetworksViewStore = create<NetworksViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-networks-view' }
  )
);
