// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { api } from '@core/api/client';
import { useLanguageStore } from '@resources/stores/language';
import type { SupportedLanguage } from '@core/i18n';

type User = {
  id: string;
  username: string;
  displayName: string | null;
  email: string | null;
  preferredLanguage: SupportedLanguage;
};

export type OIDCProvider = {
  id: string;
  name: string;
  providerType: 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0';
};

type AuthState = {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  needsSetup: boolean;
  oidcProviders: OIDCProvider[];
  checkSession: () => Promise<void>;
  login: (username: string, password: string) => Promise<{ success: boolean; error?: string }>;
  logout: () => Promise<void>;
  setup: (username: string, password: string, email?: string) => Promise<{ success: boolean; error?: string }>;
  updateProfile: (input: { displayName: string; email: string }) => Promise<{ success: boolean; error?: string }>;
  updatePreferences: (input: { preferredLanguage: SupportedLanguage }) => Promise<{ success: boolean; error?: string }>;
};

function applyUserLanguage(user: User | null) {
  if (!user?.preferredLanguage) return;
  useLanguageStore.getState().setLanguage(user.preferredLanguage);
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  isLoading: true,
  isAuthenticated: false,
  needsSetup: false,
  oidcProviders: [],

  checkSession: async () => {
    set({ isLoading: true });

    // First check public status endpoint (no auth needed)
    const statusRes = await api.get<{
      needsSetup: boolean;
      authDisabled: boolean;
      oidcProviders: OIDCProvider[];
    }>('/auth/status');

    const oidcProviders = statusRes.data?.oidcProviders ?? [];

    // If auth is disabled, create a synthetic user (check this before needsSetup)
    if (statusRes.success && statusRes.data?.authDisabled) {
      const language = useLanguageStore.getState().language;
      set({
        user: { id: 'system', username: 'admin', displayName: 'Admin', email: null, preferredLanguage: language },
        isAuthenticated: true,
        needsSetup: false,
        oidcProviders,
        isLoading: false,
      });
      return;
    }

    // If no users exist yet, redirect to setup
    if (statusRes.success && statusRes.data?.needsSetup) {
      set({ user: null, isAuthenticated: false, needsSetup: true, oidcProviders, isLoading: false });
      return;
    }

    // Try to get the current session (protected endpoint)
    const sessionRes = await api.get<User>('/auth/session');
    if (sessionRes.success && sessionRes.data) {
      applyUserLanguage(sessionRes.data);
      set({
        user: sessionRes.data,
        isAuthenticated: true,
        needsSetup: false,
        oidcProviders,
        isLoading: false,
      });
    } else {
      set({ user: null, isAuthenticated: false, needsSetup: false, oidcProviders, isLoading: false });
    }
  },

  login: async (username: string, password: string) => {
    const res = await api.post<User>('/auth/login', { username, password });
    if (res.success && res.data) {
      applyUserLanguage(res.data);
      set({ user: res.data, isAuthenticated: true });
      return { success: true };
    }
    return { success: false, error: res.error ?? 'Login failed' };
  },

  logout: async () => {
    await api.post('/auth/logout');
    set({ user: null, isAuthenticated: false });
    window.location.href = '/login';
  },

  setup: async (username: string, password: string, email?: string) => {
    const res = await api.post<User>('/auth/setup', { username, password, email });
    if (res.success && res.data) {
      applyUserLanguage(res.data);
      set({ user: res.data, isAuthenticated: true, needsSetup: false });
      return { success: true };
    }
    return { success: false, error: res.error ?? 'Setup failed' };
  },

  updateProfile: async ({ displayName, email }) => {
    const res = await api.put<User>('/auth/profile', { displayName, email });
    if (res.success && res.data) {
      set({ user: res.data });
      return { success: true };
    }
    return { success: false, error: res.error ?? 'Saving profile failed' };
  },

  updatePreferences: async ({ preferredLanguage }) => {
    useLanguageStore.getState().setLanguage(preferredLanguage);
    const res = await api.put<User>('/auth/preferences', { preferredLanguage });
    if (res.success && res.data) {
      set({ user: res.data });
      applyUserLanguage(res.data);
      return { success: true };
    }
    return { success: false, error: res.error ?? 'Saving preferences failed' };
  },
}));
