// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useLanguageStore } from '@resources/stores/language';
import type { SupportedLanguage } from '@core/i18n';
import type { User } from './authTypes';

/** Reads a login body that is the bare user rather than `{ user }`; both shapes are accepted by the server. */
export function bareUser(body: object): User | undefined {
  return 'id' in body && body.id ? (body as User) : undefined;
}

export function applyUserLanguage(user: User | null) {
  if (!user?.preferredLanguage) return;
  useLanguageStore.getState().setLanguage(user.preferredLanguage);
}

/** The stand-in account used when the server reports authentication is disabled. */
export function syntheticAdminUser(language: SupportedLanguage): User {
  return {
    id: 'system',
    username: 'admin',
    displayName: 'Admin',
    email: null,
    preferredLanguage: language,
    timeFormat: '24h',
    dateFormat: 'ddmmyyyy',
  };
}

/** Posts a JSON body with the session cookie; the login endpoints reply with flat bodies, so the raw Response is returned. */
export function postJson(path: string, body: unknown): Promise<Response> {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(body),
  });
}
