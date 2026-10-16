// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@core/i18n/i18n', () => ({
  default: { language: 'en' },
  changeAppLanguage: vi.fn(),
}));

describe('simple Zustand stores', () => {
  beforeEach(() => {
    document.documentElement.className = '';
    window.localStorage.clear();
    window.matchMedia = vi.fn().mockReturnValue({ matches: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('toggles sidebar collapsed state', async () => {
    const { useSidebarStore } = await import('./sidebar');

    useSidebarStore.setState({ collapsed: false });
    useSidebarStore.getState().toggle();

    expect(useSidebarStore.getState().collapsed).toBe(true);
  });

  it('sets header slot active state', async () => {
    const { useHeaderSlot } = await import('./headerSlot');

    useHeaderSlot.setState({ active: false });
    useHeaderSlot.getState().setActive(true);

    expect(useHeaderSlot.getState().active).toBe(true);
  });

  it('sets language and calls the i18n bridge', async () => {
    const i18nModule = await import('@core/i18n/i18n');
    const { useLanguageStore } = await import('./language');

    useLanguageStore.setState({ language: 'en' });
    useLanguageStore.getState().setLanguage('nl');

    expect(i18nModule.changeAppLanguage).toHaveBeenCalledWith('nl');
    expect(useLanguageStore.getState().language).toBe('nl');
  });

  it('resolves theme values and updates the root dark class', async () => {
    const { useThemeStore } = await import('./theme');

    useThemeStore.setState({ theme: 'dark', resolvedTheme: 'dark' });
    useThemeStore.getState().setTheme('light');

    expect(useThemeStore.getState()).toMatchObject({ theme: 'light', resolvedTheme: 'light' });
    expect(document.documentElement.classList.contains('dark')).toBe(false);

    useThemeStore.getState().setTheme('system');

    expect(useThemeStore.getState()).toMatchObject({ theme: 'system', resolvedTheme: 'dark' });
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});
