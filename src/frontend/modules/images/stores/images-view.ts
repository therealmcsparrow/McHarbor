// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type ImagesViewState = {
  viewMode: 'table' | 'card';
  setViewMode: (mode: 'table' | 'card') => void;
};

export const useImagesViewStore = create<ImagesViewState>()(
  persist(
    (set) => ({
      viewMode: 'table',
      setViewMode: (viewMode) => set({ viewMode }),
    }),
    { name: 'mcharbor-images-view' }
  )
);
