// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { motionAllowed } from './motion';

type Theme = 'light' | 'dark' | 'system';

type ThemeState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  /**
   * Flip between light and dark, revealing the new theme as a circle
   * growing from `origin` — the point the user clicked. Falls back to an
   * instant swap where the View Transition API is unavailable or motion is
   * turned down.
   */
  toggleTheme: (origin?: { x: number; y: number }) => void;
  resolvedTheme: 'light' | 'dark';
};

function resolve(theme: Theme): 'light' | 'dark' {
  if (theme !== 'system') return theme;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function apply(resolved: 'light' | 'dark') {
  document.documentElement.classList.toggle('dark', resolved === 'dark');
}

type DocumentWithViewTransition = Document & {
  startViewTransition?: (callback: () => void) => { ready: Promise<void> };
};

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: 'dark',
      resolvedTheme: 'dark',

      setTheme: (theme) => {
        const resolved = resolve(theme);
        apply(resolved);
        set({ theme, resolvedTheme: resolved });
      },

      toggleTheme: (originValue) => {
        const next: Theme = get().resolvedTheme === 'dark' ? 'light' : 'dark';
        const resolved = resolve(next);
        const doc = document as DocumentWithViewTransition;

        // One of the four sanctioned motions. Gated on both the OS setting
        // and the in-app preference — the OS setting is not always what
        // someone wants specifically here.
        if (!doc.startViewTransition || !motionAllowed()) {
          apply(resolved);
          set({ theme: next, resolvedTheme: resolved });
          return;
        }

        // The reveal origin is the toggle itself, so the new theme appears
        // to come from the control the user pressed rather than from
        // nowhere. app.css reads these in `@keyframes mc-theme-reveal`.
        const root = document.documentElement;
        root.style.setProperty('--mc-theme-x', originValue ? `${originValue.x}px` : '50%');
        root.style.setProperty('--mc-theme-y', originValue ? `${originValue.y}px` : '50%');

        doc.startViewTransition(() => {
          apply(resolved);
          set({ theme: next, resolvedTheme: resolved });
        });
      },
    }),
    { name: 'mcharbor-theme' },
  ),
);
