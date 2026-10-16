// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from './AuthProvider';
import LoginPage from './LoginPage';
import RootPage from './RootPage';
import SetupPage from './SetupPage';
import { renderWithProviders } from '../../test/render';
import { server } from '../../test/server';

const authMock = vi.hoisted(() => ({
  state: {
    isAuthenticated: false,
    needsSetup: false,
    oidcProviders: [] as Array<{
      id: string;
      name: string;
      providerType: 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0' | 'ldap';
    }>,
    checkSession: vi.fn(),
    login: vi.fn(),
    verifyMFA: vi.fn(),
    setup: vi.fn(),
  },
}));

const routerMock = vi.hoisted(() => ({
  navigate: vi.fn(),
  searchParams: new URLSearchParams(),
}));

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}));

vi.mock('./useAuth', () => ({
  useAuth: (selector: (state: typeof authMock.state) => unknown) => selector(authMock.state),
}));

vi.mock('react-router', () => ({
  Navigate: ({ to }: { to: string }) => <div data-testid="navigate">{to}</div>,
  useNavigate: () => routerMock.navigate,
  useSearchParams: () => [routerMock.searchParams],
}));

vi.mock('sonner', () => ({
  toast: toastMock,
}));

const strongPassword = 'Harbor-Pass-2026';

function databaseConfigured() {
  server.use(
    http.get('*/api/database/setup/status', () =>
      HttpResponse.json({ success: true, data: { configured: true, envPinned: false } }),
    ),
    http.get('*/api/auth/password-policy', () =>
      HttpResponse.json({
        success: true,
        data: { minLength: 12, requireUpper: true, requireLower: true, requireDigit: true, requireSymbol: true },
      }),
    ),
  );
}

describe('AuthProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    authMock.state.isAuthenticated = false;
    authMock.state.needsSetup = false;
    authMock.state.oidcProviders = [];
    routerMock.searchParams = new URLSearchParams();
  });

  it('checks the current session when mounted', () => {
    render(
      <AuthProvider>
        <div>Child</div>
      </AuthProvider>,
    );

    expect(screen.getByText('Child')).toBeInTheDocument();
    expect(authMock.state.checkSession).toHaveBeenCalledTimes(1);
  });
});

describe('RootPage', () => {
  beforeEach(() => {
    databaseConfigured();
  });

  it('routes to setup, dashboard, or login from auth state', async () => {
    authMock.state.needsSetup = true;
    const { rerender } = renderWithProviders(<RootPage />);
    expect(await screen.findByTestId('navigate')).toHaveTextContent('/setup');

    authMock.state.needsSetup = false;
    authMock.state.isAuthenticated = true;
    rerender(<RootPage />);
    expect(screen.getByTestId('navigate')).toHaveTextContent('/dashboard');

    authMock.state.isAuthenticated = false;
    rerender(<RootPage />);
    expect(screen.getByTestId('navigate')).toHaveTextContent('/login');
  });

  it('sends a fresh install to the database step first', async () => {
    server.use(
      http.get('*/api/database/setup/status', () =>
        HttpResponse.json({ success: true, data: { configured: false, envPinned: false } }),
      ),
    );
    authMock.state.needsSetup = true;
    renderWithProviders(<RootPage />);
    expect(await screen.findByTestId('navigate')).toHaveTextContent('/setup/database');
  });
});

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    databaseConfigured();
    routerMock.searchParams = new URLSearchParams();
    authMock.state.oidcProviders = [];
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: 'http://localhost:3000/login', pathname: '/login' },
    });
  });

  it('submits credentials and navigates on success', async () => {
    authMock.state.login.mockResolvedValueOnce({ success: true });
    renderWithProviders(<LoginPage />);

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'carlo' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } });
    fireEvent.submit(screen.getByRole('button', { name: 'Sign In' }).closest('form')!);

    await waitFor(() => expect(routerMock.navigate).toHaveBeenCalledWith('/', { replace: true }));
    expect(authMock.state.login).toHaveBeenCalledWith('carlo', 'secret');
  });

  it('returns to the deep link it was sent from, and only to a path inside the app', async () => {
    for (const [next, landing] of [
      ['/oncall/containers/e1/abc', '/oncall/containers/e1/abc'],
      ['//evil.example/phish', '/'],
      ['https://evil.example', '/'],
    ] as const) {
      routerMock.navigate.mockReset();
      routerMock.searchParams = new URLSearchParams({ next });
      authMock.state.login.mockResolvedValueOnce({ success: true });
      const { unmount } = renderWithProviders(<LoginPage />);
      fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'carlo' } });
      fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } });
      fireEvent.submit(screen.getByRole('button', { name: 'Sign In' }).closest('form')!);
      await waitFor(() => expect(routerMock.navigate).toHaveBeenCalledWith(landing, { replace: true }));
      unmount();
    }
  });

  it('shows OIDC errors and starts provider login', async () => {
    routerMock.searchParams = new URLSearchParams('error=oidc_failed');
    authMock.state.oidcProviders = [{ id: 'google-main', name: 'Google', providerType: 'google' }];
    renderWithProviders(<LoginPage />);

    expect(await screen.findByText('Single sign-on failed. Please try again.')).toBeInTheDocument();
    expect(toastMock.error).toHaveBeenCalledWith('Single sign-on failed. Please try again.');

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Google' }));
    expect(window.location.href).toBe('/api/identity-providers/google-main/authorize');
  });

  it('shows login failure from the store', async () => {
    authMock.state.login.mockResolvedValueOnce({ success: false, error: 'Bad credentials' });
    renderWithProviders(<LoginPage />);

    fireEvent.submit(screen.getByRole('button', { name: 'Sign In' }).closest('form')!);

    expect(await screen.findByText('Bad credentials')).toBeInTheDocument();
    expect(toastMock.error).toHaveBeenCalledWith('Bad credentials');
  });
});

describe('LoginPage directory sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    databaseConfigured();
    routerMock.searchParams = new URLSearchParams();
    authMock.state.oidcProviders = [
      { id: 'ldap-corp', name: 'Corp LDAP', providerType: 'ldap' },
      { id: 'google-main', name: 'Google', providerType: 'google' },
    ];
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: 'http://localhost:3000/login', pathname: '/login' },
    });
  });

  function signIn(username: string, password: string) {
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: username } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: password } });
    fireEvent.submit(screen.getByRole('button', { name: 'Sign In' }).closest('form')!);
  }

  it('renders an ldap provider as a directory form, not a redirect, and signs in through it', async () => {
    authMock.state.login.mockResolvedValueOnce({ success: true });
    renderWithProviders(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Corp LDAP' }));
    expect(window.location.href).toBe('http://localhost:3000/login');
    expect(screen.getByText('Directory')).toBeInTheDocument();
    expect(screen.getByText('Corp LDAP')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Forgot password?' })).not.toBeInTheDocument();

    signIn('dana', 'secret');
    await waitFor(() => expect(routerMock.navigate).toHaveBeenCalledWith('/', { replace: true }));
    expect(authMock.state.login).toHaveBeenCalledWith('dana', 'secret', 'ldap-corp');
  });

  it('offers a picker when there is more than one directory', async () => {
    authMock.state.oidcProviders = [
      { id: 'ldap-corp', name: 'Corp LDAP', providerType: 'ldap' },
      { id: 'ldap-branch', name: 'Branch AD', providerType: 'ldap' },
    ];
    authMock.state.login.mockResolvedValueOnce({ success: true });
    renderWithProviders(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with a directory account' }));
    fireEvent.click(screen.getByRole('button', { name: 'Directory' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Branch AD' }));

    signIn('dana', 'secret');
    await waitFor(() => expect(authMock.state.login).toHaveBeenCalledWith('dana', 'secret', 'ldap-branch'));
  });

  it('takes a directory sign-in that owes a second factor to the MFA step', async () => {
    authMock.state.login.mockResolvedValueOnce({
      success: false,
      error: '',
      mfaRequired: true,
      challengeId: 'challenge-1',
    });
    renderWithProviders(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Corp LDAP' }));
    signIn('dana', 'secret');

    expect(await screen.findByLabelText('Authentication code')).toBeInTheDocument();
    expect(routerMock.navigate).not.toHaveBeenCalled();
  });

  it('shows a locked directory account the same notice as a local one', async () => {
    authMock.state.login.mockResolvedValueOnce({
      success: false,
      error: 'Account temporarily locked due to too many failed attempts',
      locked: true,
      retryAfterSeconds: 60,
    });
    renderWithProviders(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Corp LDAP' }));
    signIn('dana', 'wrong');

    expect(await screen.findByText('Account temporarily locked')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Try again in 1:00' })).toBeDisabled();
  });

  it('returns to the local form without carrying the directory over', async () => {
    authMock.state.login.mockResolvedValueOnce({ success: true });
    renderWithProviders(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in with Corp LDAP' }));
    fireEvent.click(screen.getByRole('button', { name: 'Use a McHarbor account' }));
    expect(screen.getByRole('button', { name: 'Forgot password?' })).toBeInTheDocument();

    signIn('carlo', 'secret');
    await waitFor(() => expect(authMock.state.login).toHaveBeenCalledWith('carlo', 'secret'));
  });
});

describe('SetupPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    databaseConfigured();
  });

  async function fillPasswords(password: string, confirm: string) {
    fireEvent.change(await screen.findByLabelText('Password'), { target: { value: password } });
    fireEvent.change(screen.getByLabelText('Confirm Password'), { target: { value: confirm } });
    fireEvent.submit(screen.getByRole('button', { name: 'Create Admin Account' }).closest('form')!);
  }

  it('validates password mismatch and the password policy before submitting', async () => {
    renderWithProviders(<SetupPage />);

    await fillPasswords(strongPassword, `${strongPassword}x`);
    expect(await screen.findByText('Passwords do not match')).toBeInTheDocument();

    await fillPasswords('short', 'short');
    expect(await screen.findByText('This password does not meet the password policy.')).toBeInTheDocument();
    expect(authMock.state.setup).not.toHaveBeenCalled();
  });

  it('submits setup data and handles failure', async () => {
    authMock.state.setup.mockResolvedValueOnce({ success: true });
    renderWithProviders(<SetupPage />);

    fireEvent.change(await screen.findByLabelText('Email (optional)'), { target: { value: 'admin@example.test' } });
    await fillPasswords(strongPassword, strongPassword);

    await waitFor(() => expect(routerMock.navigate).toHaveBeenCalledWith('/dashboard'));
    expect(authMock.state.setup).toHaveBeenCalledWith('admin', strongPassword, 'admin@example.test');

    authMock.state.setup.mockResolvedValueOnce({ success: false, error: 'Cannot create admin' });
    await fillPasswords(strongPassword, strongPassword);

    expect(await screen.findByText('Cannot create admin')).toBeInTheDocument();
    expect(toastMock.error).toHaveBeenCalledWith('Cannot create admin');
  });
});
