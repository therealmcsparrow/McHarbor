// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useThemeStore } from '@resources/stores/theme';

const ThemeContext = createContext<null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const theme = useThemeStore((s) => s.theme);

  useEffect(() => {
    const resolved =
      theme === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
          ? 'dark'
          : 'light'
        : theme;

    document.documentElement.classList.toggle('dark', resolved === 'dark');
  }, [theme]);

  return <ThemeContext.Provider value={null}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  useContext(ThemeContext);
  return useThemeStore();
}
