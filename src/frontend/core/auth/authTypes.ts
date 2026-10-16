// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { SupportedLanguage } from '@core/i18n';

/** The signed-in account, as `/auth/status` and `/auth/preferences` return it. */
export type User = {
  id: string;
  username: string;
  displayName: string | null;
  email: string | null;
  preferredLanguage: SupportedLanguage;
  // identityProviderId is the row id of the identity_providers
  // record the user signed in with, or empty for local accounts.
  // When set, the profile page renders displayName and email as
  // read-only because the IdP owns those fields.
  identityProviderId?: string;
  timeFormat: '12h' | '24h';
  dateFormat: 'ddmmyyyy' | 'mmddyyyy';
  // Toast overrides. Absent means "inherit the app-wide default" — which is
  // not the same as a zero value, because 0 is a legal duration ("never
  // auto-dismiss") and false is a legal richColors. The backend omits each
  // field entirely rather than sending null, so `undefined` is the only
  // "not set" this type has to model.
  toastPosition?: ToastPosition;
  toastDurationMs?: number;
  toastMaxVisible?: number;
  toastRichColors?: boolean;
  toastCloseButton?: boolean;
  // Appearance overrides, same contract: absent means "inherit the app-wide
  // default", and every one of them is ignored while the operator has turned
  // user overrides off.
  appearanceThemeId?: string;
  appearanceAccent?: string;
  appearanceFont?: string;
  appearanceBaseFontPx?: number;
};

/** The six slots sonner can place the toaster in. */
export type ToastPosition = 'top-left' | 'top-center' | 'top-right' | 'bottom-left' | 'bottom-center' | 'bottom-right';

export type OIDCProvider = {
  id: string;
  name: string;
  providerType: 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0' | 'ldap';
};

export type LoginResult =
  | { success: true }
  | {
      success: false;
      error: string;
      locked?: boolean;
      disabled?: boolean;
      mustResetPassword?: boolean;
      retryAfterSeconds?: number;
      /**
       * The password was correct but the account carries a second
       * factor. No session exists yet — the caller must complete
       * `verifyMFA(challengeId, code)` within the challenge's
       * five-minute window.
       */
      mfaRequired?: boolean;
      challengeId?: string;
    };

export type AuthState = {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  needsSetup: boolean;
  oidcProviders: OIDCProvider[];
  mustResetPassword: boolean;
  mfaEnrollmentRequired: boolean;
  /**
   * The password the user most recently logged in with. Cleared
   * after the forced-reset flow consumes it. Held in memory only —
   * never written to localStorage / sessionStorage. Used so the
   * forced-reset screen can satisfy the "current password" check
   * without prompting the user to type it again.
   */
  lastLoginPassword: string | null;
  checkSession: () => Promise<void>;
  login: (username: string, password: string, directoryProviderId?: string) => Promise<LoginResult>;
  verifyMFA: (challengeId: string, code: string) => Promise<LoginResult>;
  logout: () => void;
  reset: () => void;
  setup: (username: string, password: string, email?: string) => Promise<{ success: boolean; error?: string }>;
  updateProfile: (input: { displayName: string; email: string }) => Promise<{ success: boolean; error?: string }>;
  updatePreferences: (input: { preferredLanguage: SupportedLanguage }) => Promise<{ success: boolean; error?: string }>;
  clearMustResetPassword: () => void;
  clearMFAEnrollmentRequired: () => void;
  consumeLastLoginPassword: () => string | null;
};

export type LoginPayload = {
  user?: User;
  mustResetPassword?: boolean;
  mfaEnrollmentRequired?: boolean;
  mfaRequired?: boolean;
  challengeId?: string;
  success?: false;
  error?: string;
  locked?: boolean;
  disabled?: boolean;
  retryAfterSeconds?: number;
};

export type MFAPayload = {
  user?: User;
  mustResetPassword?: boolean;
  error?: string;
  code?: string;
};
