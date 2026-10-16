// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type DemoBannerState = {
  dismissedFor: string | null;
  dismiss: (visit: string) => void;
  show: () => void;
};

export const useDemoBannerStore = create<DemoBannerState>()(
  persist(
    (set) => ({
      dismissedFor: null,
      dismiss: (visit) => set({ dismissedFor: visit }),
      show: () => set({ dismissedFor: null }),
    }),
    { name: 'mcharbor-demo-banner' },
  ),
);

export function demoVisitKey(enteredAt: string | undefined): string {
  return enteredAt ?? 'active';
}
