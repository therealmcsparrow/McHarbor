// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';

type WhatsNewState = {
  open: boolean;
  show: () => void;
  hide: () => void;
};

/** Lets any page reopen the release highlights the layout's overlay renders. */
export const useWhatsNewStore = create<WhatsNewState>()((set) => ({
  open: false,
  show: () => set({ open: true }),
  hide: () => set({ open: false }),
}));
