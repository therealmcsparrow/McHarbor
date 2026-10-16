// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ApiResponse } from '@core/api/client';
import { useLanguageStore } from '@resources/stores/language';
import { useAuth } from './useAuth';
import { server } from '../../test/server';

const locationReplace = vi.fn();

const apiMock = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}));

const languageMock = vi.hoisted(() => ({
  state: {
    language: 'en',
    setLanguage: vi.fn((language: string) => {
      languageMock.state.language = language;
    }),
  },
  setState: (state: { language?: string }) => {
    if (state.language) languageMock.state.language = state.language;
    languageMock.state.setLanguage.mockClear();
  },
}));

vi.mock('@core/api/client', () => ({
  api: apiMock,
}));

vi.mock('@resources/stores/language', () => ({
  useLanguageStore: {
    getState: () => languageMock.state,
    setState: languageMock.setState,
  },
}));

const user = {
  id: 'usr_1',
  username: 'carlo',
  displayName: 'Carlo',
  email: 'carlo@example.test',
  preferredLanguage: 'nl' as const,
};

function ok<T>(data: T): ApiResponse<T> {
  return { success: true, data };
}

function fail(error?: string): ApiResponse {
  return { success: false, error };
}

describe('useAuth store', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuth.setState({
      user: null,
      isLoading: true,
      isAuthenticated: false,
      needsSetup: false,
      oidcProviders: [],
    });
    useLanguageStore.setState({ language: 'en' });
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: 'http://localhost:3000/', pathname: '/', replace: locationReplace },
    });
  });

  it('creates a synthetic user when auth is disabled', async () => {
    useLanguageStore.setState({ language: 'de' });
    apiMock.get.mockResolvedValueOnce(ok({ needsSetup: false, authDisabled: true, oidcProviders: [] }));

    await useAuth.getState().checkSession();

    expect(useAuth.getState().isAuthenticated).toBe(true);
    expect(useAuth.getState().user?.id).toBe('system');
    expect(useAuth.getState().user?.preferredLanguage).toBe('de');
    expect(apiMock.get).toHaveBeenCalledTimes(1);
  });

  it('detects setup mode before fetching a protected session', async () => {
    apiMock.get.mockResolvedValueOnce(
      ok({
        needsSetup: true,
        authDisabled: false,
        oidcProviders: [{ id: 'oidc', name: 'OIDC', providerType: 'generic_oidc' }],
      }),
    );

    await useAuth.getState().checkSession();

    expect(useAuth.getState().needsSetup).toBe(true);
    expect(useAuth.getState().oidcProviders).toHaveLength(1);
    expect(apiMock.get).toHaveBeenCalledTimes(1);
  });

  it('loads an existing session and applies the user language', async () => {
    apiMock.get
      .mockResolvedValueOnce(ok({ needsSetup: false, authDisabled: false, oidcProviders: [] }))
      .mockResolvedValueOnce(ok(user));

    await useAuth.getState().checkSession();

    expect(useAuth.getState().user).toEqual(user);
    expect(useAuth.getState().isAuthenticated).toBe(true);
    expect(useLanguageStore.getState().language).toBe('nl');
  });

  it('marks the user anonymous when the session lookup fails', async () => {
    apiMock.get
      .mockResolvedValueOnce(ok({ needsSetup: false, authDisabled: false, oidcProviders: [] }))
      .mockResolvedValueOnce(fail('Unauthorized'));

    await useAuth.getState().checkSession();

    expect(useAuth.getState().user).toBeNull();
    expect(useAuth.getState().isAuthenticated).toBe(false);
    expect(useAuth.getState().isLoading).toBe(false);
  });

  it('handles login, setup, profile, preference, and logout mutations', async () => {
    server.use(http.post('*/api/auth/login', () => HttpResponse.json({ user }), { once: true }));
    await expect(useAuth.getState().login('carlo', 'secret')).resolves.toEqual({ success: true });
    expect(useAuth.getState().isAuthenticated).toBe(true);

    server.use(
      http.post('*/api/auth/login', () => HttpResponse.json({ success: false }, { status: 401 }), { once: true }),
    );
    await expect(useAuth.getState().login('bad', 'bad')).resolves.toEqual({
      success: false,
      error: 'Login failed',
      locked: false,
      disabled: false,
      retryAfterSeconds: undefined,
    });

    apiMock.post.mockResolvedValueOnce(ok(user));
    await expect(useAuth.getState().setup('carlo', 'password', 'carlo@example.test')).resolves.toEqual({
      success: true,
    });

    apiMock.put.mockResolvedValueOnce(ok({ ...user, displayName: 'C' }));
    await expect(useAuth.getState().updateProfile({ displayName: 'C', email: 'c@example.test' })).resolves.toEqual({
      success: true,
    });
    expect(useAuth.getState().user?.displayName).toBe('C');

    apiMock.put.mockResolvedValueOnce(ok({ ...user, preferredLanguage: 'de' as const }));
    await expect(useAuth.getState().updatePreferences({ preferredLanguage: 'de' })).resolves.toEqual({ success: true });
    expect(useLanguageStore.getState().language).toBe('de');

    apiMock.put.mockResolvedValueOnce(fail());
    await expect(useAuth.getState().updatePreferences({ preferredLanguage: 'en' })).resolves.toEqual({
      success: false,
      error: 'Saving preferences failed',
    });

    apiMock.post.mockResolvedValueOnce(ok({}));
    useAuth.getState().logout();

    expect(useAuth.getState().isAuthenticated).toBe(false);
    expect(locationReplace).toHaveBeenCalledWith('/login');
  });
  function directoryDeclared(declared: boolean) {
    useAuth.setState({ oidcProviders: [{ id: 'ldap-corp', name: 'Corp', providerType: 'ldap' }] });
    apiMock.get.mockImplementation(async (path: string) =>
      path === '/plugins/loader/login-endpoints'
        ? ok(declared ? [{ providerType: 'ldap', path: '/api/identity-ldap/auth' }] : [])
        : fail(),
    );
  }

  it('sends a directory sign-in to the route its plugin declares, with its provider', async () => {
    directoryDeclared(true);
    let sent: unknown;
    server.use(
      http.post(
        '*/api/identity-ldap/auth',
        async ({ request }) => {
          sent = await request.json();
          return HttpResponse.json({ user, mfaEnrollmentRequired: true });
        },
        { once: true },
      ),
    );
    await expect(useAuth.getState().login('dana', 'secret', 'ldap-corp')).resolves.toEqual({ success: true });
    expect(sent).toEqual({ providerId: 'ldap-corp', username: 'dana', password: 'secret' });
    expect(useAuth.getState().isAuthenticated).toBe(true);
    expect(useAuth.getState().mfaEnrollmentRequired).toBe(true);
  });

  it('refuses a directory sign-in without posting when no plugin declares its login endpoint', async () => {
    directoryDeclared(false);
    const posted = vi.fn();
    server.use(
      http.post('*/api/identity-ldap/auth', () => {
        posted();
        return HttpResponse.json({ user });
      }),
    );
    await expect(useAuth.getState().login('dana', 'secret', 'ldap-corp')).resolves.toEqual({
      success: false,
      error: 'Login failed',
    });
    expect(posted).not.toHaveBeenCalled();
    expect(useAuth.getState().isAuthenticated).toBe(false);
  });

  it('reads the MFA challenge and the lockout from a directory sign-in like a local one', async () => {
    directoryDeclared(true);
    server.use(
      http.post(
        '*/api/identity-ldap/auth',
        () => HttpResponse.json({ mfaRequired: true, challengeId: 'challenge-1' }),
        { once: true },
      ),
    );
    await expect(useAuth.getState().login('dana', 'secret', 'ldap-corp')).resolves.toEqual({
      success: false,
      error: '',
      mfaRequired: true,
      challengeId: 'challenge-1',
    });

    server.use(
      http.post(
        '*/api/identity-ldap/auth',
        () =>
          HttpResponse.json(
            {
              success: false,
              error: 'Account temporarily locked due to too many failed attempts',
              locked: true,
              retryAfterSeconds: 60,
            },
            { status: 423 },
          ),
        { once: true },
      ),
    );
    await expect(useAuth.getState().login('dana', 'wrong', 'ldap-corp')).resolves.toEqual({
      success: false,
      error: 'Account temporarily locked due to too many failed attempts',
      locked: true,
      disabled: false,
      retryAfterSeconds: 60,
    });
  });
});
