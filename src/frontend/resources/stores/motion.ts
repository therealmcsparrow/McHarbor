// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Motion preference.
//
// `prefers-reduced-motion` is the floor, not the ceiling. Someone can be
// perfectly happy with motion in their OS generally and still not want a
// status dot pulsing in their peripheral vision on a dashboard they leave
// open all day — and the reverse is true too: a user with the OS setting on
// for accessibility reasons elsewhere may still want the theme reveal here.
//
// So there are three states:
//
//   'system'  follow prefers-reduced-motion (default)
//   'full'    always animate, even if the OS says reduce
//   'reduced' never animate, even if the OS says otherwise
//
// The resolved answer is written to `data-reduce-motion` on <html>, which
// `app.css` pairs with its own `@media (prefers-reduced-motion)` block.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type MotionPreference = 'system' | 'full' | 'reduced';

type MotionState = {
  preference: MotionPreference;
  setPreference: (preference: MotionPreference) => void;
  /** Apply the current preference to the document. Call once at boot. */
  sync: () => void;
};

function systemPrefersReduced(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

export function resolveReduced(preference: MotionPreference): boolean {
  if (preference === 'reduced') return true;
  if (preference === 'full') return false;
  return systemPrefersReduced();
}

function apply(preference: MotionPreference) {
  if (typeof document === 'undefined') return;
  const reduced = resolveReduced(preference);
  // 'full' must actively override the OS media query, so the attribute is
  // written in all three states rather than only when reducing.
  document.documentElement.dataset.reduceMotion = reduced ? 'true' : 'false';
  document.documentElement.dataset.motionPreference = preference;
}

export const useMotionStore = create<MotionState>()(
  persist(
    (set, get) => ({
      preference: 'system',
      setPreference: (preference) => {
        apply(preference);
        set({ preference });
      },
      sync: () => apply(get().preference),
    }),
    {
      name: 'mcharbor-motion',
      onRehydrateStorage: () => (state) => state?.sync(),
    },
  ),
);

/**
 * Imperative check for code outside React — the theme store's view
 * transition, for instance, which runs before any component renders.
 */
export function motionAllowed(): boolean {
  return !resolveReduced(useMotionStore.getState().preference);
}
