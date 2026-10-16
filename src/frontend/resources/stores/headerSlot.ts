// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';

type HeaderSlotStore = {
  /** How many page heads are mounted right now. */
  count: number;
  active: boolean;
  setActive: (active: boolean) => void;
};

// Ref-counted rather than a plain boolean: React mounts the incoming page
// before unmounting the outgoing one in some transitions, and a page may hold
// a head while a child renders its own. A last-writer-wins flag turns the page
// gutter back on underneath a head that is still there.
export const useHeaderSlot = create<HeaderSlotStore>((set) => ({
  count: 0,
  active: false,
  setActive: (active) =>
    set((state) => {
      const count = Math.max(0, state.count + (active ? 1 : -1));
      return { count, active: count > 0 };
    }),
}));
