// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';

type HeaderSlotStore = {
  active: boolean;
  setActive: (active: boolean) => void;
};

export const useHeaderSlot = create<HeaderSlotStore>((set) => ({
  active: false,
  setActive: (active) => set({ active }),
}));
