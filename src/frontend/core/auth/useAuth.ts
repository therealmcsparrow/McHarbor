// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { api } from '@core/api/client';
import { clearReadCache } from '@core/pwa/readCache';
import { fetchLoginEndpoint } from '@core/plugins/capabilities';
import { useLanguageStore } from '@resources/stores/language';
import type { AuthState, LoginPayload, MFAPayload, OIDCProvider, User } from './authTypes';
import { applyUserLanguage, bareUser, postJson, syntheticAdminUser } from './authHelpers';
import { isSetupTokenRefusal, setupTokenHeaders, setupTokenRefusalMessage } from './setupToken';

export type { OIDCProvider, ToastPosition, User } from './authTypes';

export const useAuth = create<AuthState>((set, get) => ({
  user: null,
  isLoading: true,
  isAuthenticated: false,
  needsSetup: false,
  oidcProviders: [],
  mustResetPassword: false,
  mfaEnrollmentRequired: false,
  lastLoginPassword: null,

  clearMFAEnrollmentRequired: () => set({ mfaEnrollmentRequired: false }),

  clearMustResetPassword: () => set({ mustResetPassword: false, lastLoginPassword: null }),

  /**
   * Returns the in-memory password exactly once and then clears it.
   * The forced-reset screen calls this on submit so a refresh of
   * the page forces the user back to login (intentional — the
   * forced-reset is a security checkpoint).
   */
  consumeLastLoginPassword: () => {
    const pw = get().lastLoginPassword;
    if (pw) set({ lastLoginPassword: null });
    return pw;
  },

  checkSession: async () => {
    // Snapshot the current state up front — if the user has already
    // logged in (via `login()` racing ahead of this fetch), we must
    // NOT clobber the authenticated state when the /auth/session call
    // returns 401 because the in-flight request was issued before the
    // session cookie was set. Otherwise the AppLayout, which mounts
    // after `login()` navigates to /dashboard, would see
    // isAuthenticated: false on its first render and bounce back to
    // /login. Hard-reload works because it starts a fresh page-load
    // sequence where checkSession's first request already carries the
    // cookie.
    const wasAuthenticated = get().isAuthenticated;
    const knownUser = get().user;
    set({ isLoading: true });

    // First check public status endpoint (no auth needed)
    const statusRes = await api.get<{
      needsSetup: boolean;
      authDisabled: boolean;
      oidcProviders: OIDCProvider[];
      defaultLanguage?: string;
    }>('/auth/status');

    const oidcProviders = statusRes.data?.oidcProviders ?? [];

    // If auth is disabled, create a synthetic user (check this before needsSetup)
    if (statusRes.success && statusRes.data?.authDisabled) {
      const language = useLanguageStore.getState().language;
      set({
        user: syntheticAdminUser(language),
        isAuthenticated: true,
        needsSetup: false,
        oidcProviders,
        isLoading: false,
      });
      return;
    }

    // If no users exist yet, redirect to setup — but only when we
    // weren't already authenticated (otherwise a logged-in user
    // racing the auth check would be redirected to /setup).
    if (statusRes.success && statusRes.data?.needsSetup && !wasAuthenticated) {
      set({ user: null, isAuthenticated: false, needsSetup: true, oidcProviders, isLoading: false });
      return;
    }

    // Try to get the current session (protected endpoint)
    const sessionRes = await api.get<User & { mfaEnrollmentRequired?: boolean }>('/auth/session');
    if (sessionRes.success && sessionRes.data) {
      const { mfaEnrollmentRequired, ...sessionUser } = sessionRes.data;
      applyUserLanguage(sessionUser);
      set({
        user: sessionUser,
        isAuthenticated: true,
        mfaEnrollmentRequired: Boolean(mfaEnrollmentRequired),
        needsSetup: false,
        oidcProviders,
        isLoading: false,
      });
    } else if (wasAuthenticated && knownUser) {
      // The session probe failed but the user has already logged in
      // during this page-load sequence (login() resolved while we
      // were waiting for /auth/status). Trust the existing state —
      // the next legitimate /auth/session check (page reload, manual
      // re-check) will reconcile if the cookie truly is gone.
      set({ isLoading: false });
    } else {
      set({ user: null, isAuthenticated: false, needsSetup: false, oidcProviders, isLoading: false });
    }
  },

  login: async (username: string, password: string, directoryProviderId?: string) => {
    // Use a raw call because the lockout / disabled / forced-reset
    // surfaces carry extra fields that the typed ApiResponse<User>
    // would silently drop. We only know which shape comes back by
    // reading the HTTP status code.
    //
    // A directory provider signs in through whichever loaded plugin declares a
    // login-endpoint for its type; with that plugin gone, the sign-in fails
    // instead of posting credentials to a route nobody serves.
    let loginPath = '/api/auth/login';
    if (directoryProviderId) {
      const provider = get().oidcProviders.find((p) => p.id === directoryProviderId);
      const endpoint = provider ? await fetchLoginEndpoint(provider.providerType).catch(() => undefined) : undefined;
      if (!endpoint) return { success: false, error: 'Login failed' };
      loginPath = endpoint.path;
    }
    const res = await postJson(
      loginPath,
      directoryProviderId ? { providerId: directoryProviderId, username, password } : { username, password },
    );

    const data = (await res.json().catch(() => ({}))) as LoginPayload;

    if (res.ok && data.mfaRequired && data.challengeId) {
      // Password accepted, second factor outstanding. No cookie was
      // set, so we stay signed out and hand the challenge back.
      //
      // The password is stashed for the same reason the forced-reset
      // branch below stashes it: if this account also owes a password
      // rotation, the change screen needs the current password and
      // has no other way to get it. verifyMFA drops it again the
      // moment it turns out not to be needed.
      set({ lastLoginPassword: password });
      return {
        success: false,
        error: '',
        mfaRequired: true,
        challengeId: data.challengeId,
      };
    }

    if (res.ok) {
      // Success body may be the bare user, or { user, mustResetPassword }.
      // unavoidable cast: the body is read straight off res.json(), so it
      // arrives as `unknown`. Both shapes are checked below by `user.id`.
      const user = data.user ?? bareUser(data);
      if (user && user.id) {
        applyUserLanguage(user);
        const mustReset = Boolean(data.mustResetPassword);
        set({
          user,
          isAuthenticated: true,
          mustResetPassword: mustReset,
          mfaEnrollmentRequired: Boolean(data.mfaEnrollmentRequired),
          // Only stash the password when the server demands a
          // forced-reset — otherwise we'd be holding a credential
          // for no reason.
          lastLoginPassword: mustReset ? password : null,
        });
        return mustReset ? { success: true, mustResetPassword: true } : { success: true };
      }
      return { success: false, error: 'Login failed' };
    }

    return {
      success: false,
      error: data.error ?? 'Login failed',
      locked: Boolean(data.locked),
      disabled: Boolean(data.disabled),
      retryAfterSeconds: data.retryAfterSeconds,
    };
  },

  verifyMFA: async (challengeId: string, code: string) => {
    // Raw call for the same reason login() uses one: the success body
    // is the flat `{user, mustResetPassword}` shape, not the standard
    // `{success, data}` envelope.
    const res = await postJson('/api/auth/login/mfa', { challengeId, code });

    const data = (await res.json().catch(() => ({}))) as MFAPayload;

    if (res.ok && data.user?.id) {
      const user = data.user;
      applyUserLanguage(user);
      const mustReset = Boolean(data.mustResetPassword);
      set({
        user,
        isAuthenticated: true,
        mustResetPassword: mustReset,
        // Release the password stashed by the challenge branch unless
        // the forced-reset screen still needs it.
        lastLoginPassword: mustReset ? get().lastLoginPassword : null,
      });
      return mustReset ? { success: true, mustResetPassword: true } : { success: true };
    }

    return {
      success: false as const,
      error: data.error ?? 'Verification failed',
      // A dead challenge means the whole login has to start over; the
      // code being merely wrong does not.
      challengeId: res.status === 401 && data.code === 'err.mfa.challenge_invalid' ? undefined : challengeId,
    };
  },

  logout: () => {
    void clearReadCache();
    // Clear local state and navigate immediately so the user is
    // "out" regardless of the API call outcome. The cookie clearing
    // runs in the background with a timeout — abort so
    // it can't hang the tab — and any failure is swallowed because
    // the session is already destroyed from the user's perspective.
    set({
      user: null,
      isAuthenticated: false,
      mustResetPassword: false,
      mfaEnrollmentRequired: false,
      lastLoginPassword: null,
    });
    window.location.replace('/login');

    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 5000);
    void api
      .post('/auth/logout', undefined, { signal: controller.signal })
      .catch(() => undefined)
      .finally(() => window.clearTimeout(timeout));
  },

  // Client-side state reset only — the server has already wiped
  // auth/users in response to the POST /auth/reset that just
  // succeeded. Force a full reload so the next /auth/status check
  // re-evaluates needsSetup and routes the operator back through
  // the first-run setup flow.
  reset: () => {
    set({
      user: null,
      isAuthenticated: false,
      mustResetPassword: false,
      mfaEnrollmentRequired: false,
      lastLoginPassword: null,
      needsSetup: true,
    });
    window.location.href = '/login';
  },

  setup: async (username: string, password: string, email?: string) => {
    const res = await api.post<User>('/auth/setup', { username, password, email }, { headers: setupTokenHeaders() });
    if (res.success && res.data) {
      applyUserLanguage(res.data);
      set({ user: res.data, isAuthenticated: true, needsSetup: false });
      return { success: true };
    }
    if (isSetupTokenRefusal(res)) return { success: false, error: setupTokenRefusalMessage() };
    return { success: false, error: res.error };
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
