// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useThemeStore } from '@resources/stores/theme';
import { useApplyThemeTokens } from './useApplyThemeTokens';

const ThemeContext = createContext<null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const theme = useThemeStore((s) => s.theme);

  // Loads the active theme stylesheet, applies the .dark variant class,
  // and pushes per-theme CSS variable overrides. Must be called once
  // at app mount; subsequent state changes from the Themes tab
  // re-trigger the effects inside the hook.
  useApplyThemeTokens();

  // The single writer of `.dark`. `useApplyThemeTokens` deliberately does
  // not touch the class - two writers meant the topbar toggle and the
  // Settings variant buttons each undid the other.
  useEffect(() => {
    const query = window.matchMedia('(prefers-color-scheme: dark)');

    const apply = () => {
      const resolved = theme === 'system' ? (query.matches ? 'dark' : 'light') : theme;
      document.documentElement.classList.toggle('dark', resolved === 'dark');
      if (useThemeStore.getState().resolvedTheme !== resolved) {
        useThemeStore.setState({ resolvedTheme: resolved });
      }
    };

    apply();
    if (theme !== 'system') return;

    // Only 'system' follows the OS, and it has to follow it live - a user
    // whose machine flips at sunset should not have to reload.
    query.addEventListener('change', apply);
    return () => query.removeEventListener('change', apply);
  }, [theme]);

  return <ThemeContext.Provider value={null}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  useContext(ThemeContext);
  return useThemeStore();
}
