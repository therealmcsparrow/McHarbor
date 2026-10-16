// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from '@core/i18n/i18n';
import type { ApiResponse } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Must match `firstrun.Header` in src/backend/core/firstrun/firstrun.go. */
export const SETUP_TOKEN_HEADER = 'X-McHarbor-Setup-Token';

const STORAGE_KEY = 'mcharbor-setup-token';

export function readSetupToken(): string {
  try {
    return sessionStorage.getItem(STORAGE_KEY) ?? '';
  } catch {
    // Storage blocked (private window, previews): the field keeps the value for this page.
    return '';
  }
}

export function writeSetupToken(value: string): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, value.trim());
  } catch {
    // Storage blocked: nothing to persist, and the installer still works within the page.
  }
}

export function setupTokenHeaders(): Record<string, string> {
  const token = readSetupToken();
  return token ? { [SETUP_TOKEN_HEADER]: token } : {};
}

export function isSetupTokenRefusal(res: Pick<ApiResponse<unknown>, 'success' | 'data'>): boolean {
  return !res.success && (res.data as { reason?: unknown } | undefined)?.reason === 'setup_token';
}

export function setupTokenRefusalMessage(): string {
  return i18n.t('setupToken.invalid', { ns: 'common' });
}

/**
 * assertSuccess for the first-run routes: a missing or stale token becomes the
 * one message that tells the operator where to find the current token.
 */
export function setupRefusal<T>(res: ApiResponse<T>): T {
  if (isSetupTokenRefusal(res)) throw new Error(setupTokenRefusalMessage());
  return assertSuccess(res);
}
