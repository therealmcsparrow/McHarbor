// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuth, type User } from '@core/auth/useAuth';
import { MFAEnrollmentGuard } from '@core/auth/MFAEnrollmentGuard';
import MFAEnrollmentRequiredPage from './MFAEnrollmentRequiredPage';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const routerMock = vi.hoisted(() => ({ navigate: vi.fn() }));

vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>();
  return { ...actual, useNavigate: () => routerMock.navigate };
});

const heldUser = { id: 'u1', username: 'operator' } as User;
const logout = vi.fn();

function holdSession(enrollmentRequired: boolean, user: User | null = heldUser) {
  useAuth.setState({
    user,
    isAuthenticated: user !== null,
    isLoading: false,
    mfaEnrollmentRequired: enrollmentRequired,
    logout,
  });
}

function renderGuarded() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/dashboard']}>
      <Routes>
        <Route
          path="/dashboard"
          element={
            <MFAEnrollmentGuard>
              <div>app shell</div>
            </MFAEnrollmentGuard>
          }
        />
        <Route path="/mfa-enrollment" element={<div>enrolment screen</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('MFA enrolment enforcement', () => {
  beforeEach(() => {
    routerMock.navigate.mockReset();
    logout.mockReset();
  });

  afterEach(() => {
    useAuth.setState({ user: null, isAuthenticated: false, mfaEnrollmentRequired: false });
  });

  it('routes a held session away from the app shell to the enrolment screen', () => {
    holdSession(true);
    renderGuarded();
    expect(screen.getByText('enrolment screen')).toBeInTheDocument();
    expect(screen.queryByText('app shell')).not.toBeInTheDocument();
  });

  it('lets a session that owes nothing into the app shell', () => {
    holdSession(false);
    renderGuarded();
    expect(screen.getByText('app shell')).toBeInTheDocument();
  });

  it('enrols, clears the hold and continues to the dashboard', async () => {
    holdSession(true);
    const confirmed: unknown[] = [];
    server.use(
      http.post('*/api/auth/mfa/enroll/begin', () =>
        HttpResponse.json({
          success: true,
          data: {
            secret: 'JBSWY3DPEHPK3PXP',
            otpauthUri: 'otpauth://totp/McHarbor:operator?secret=JBSWY3DPEHPK3PXP',
            recoveryCodes: ['aaaa-bbbb'],
          },
        }),
      ),
      http.post('*/api/auth/mfa/enroll/confirm', async ({ request }) => {
        confirmed.push(await request.json());
        return HttpResponse.json({ success: true, data: { ok: true } });
      }),
    );

    renderWithProviders(<MFAEnrollmentRequiredPage />);
    expect(screen.getByText('Two-factor authentication required')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Set up two-factor authentication' }));
    const codeInput = await screen.findByLabelText('3. Enter the code from your app');
    fireEvent.change(codeInput, { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Verify and turn on' }));

    await waitFor(() => expect(routerMock.navigate).toHaveBeenCalledWith('/dashboard', { replace: true }));
    expect(confirmed).toEqual([{ code: '123456' }]);
    expect(useAuth.getState().mfaEnrollmentRequired).toBe(false);
  });

  it('keeps the hold and says so when setup cannot start', async () => {
    holdSession(true);
    server.use(
      http.post('*/api/auth/mfa/enroll/begin', () =>
        HttpResponse.json({ success: false, error: 'boom', code: 'err.mfa.enroll_failed' }, { status: 500 }),
      ),
    );

    renderWithProviders(<MFAEnrollmentRequiredPage />);
    fireEvent.click(screen.getByRole('button', { name: 'Set up two-factor authentication' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not start setup. Please try again.');
    expect(useAuth.getState().mfaEnrollmentRequired).toBe(true);
    expect(routerMock.navigate).not.toHaveBeenCalledWith('/dashboard', { replace: true });
  });

  it('leaves the screen when nothing is owed, and sends a signed-out visitor to login', () => {
    holdSession(false);
    const { unmount } = renderWithProviders(<MFAEnrollmentRequiredPage />);
    expect(routerMock.navigate).toHaveBeenCalledWith('/dashboard', { replace: true });
    unmount();

    routerMock.navigate.mockReset();
    holdSession(true, null);
    renderWithProviders(<MFAEnrollmentRequiredPage />);
    expect(routerMock.navigate).toHaveBeenCalledWith('/login', { replace: true });
  });

  it('reads the hold back from the session on reload, without leaking the flag into the user', async () => {
    useAuth.setState({ user: null, isAuthenticated: false, mfaEnrollmentRequired: false });
    server.use(
      http.get('*/api/auth/status', () =>
        HttpResponse.json({ success: true, data: { needsSetup: false, authDisabled: false, oidcProviders: [] } }),
      ),
      http.get('*/api/auth/session', () =>
        HttpResponse.json({ success: true, data: { id: 'u1', username: 'operator', mfaEnrollmentRequired: true } }),
      ),
    );

    await useAuth.getState().checkSession();

    expect(useAuth.getState().isAuthenticated).toBe(true);
    expect(useAuth.getState().mfaEnrollmentRequired).toBe(true);
    expect(useAuth.getState().user).not.toHaveProperty('mfaEnrollmentRequired');
  });

  it('offers sign-out as the only other way off the screen', () => {
    holdSession(true);
    renderWithProviders(<MFAEnrollmentRequiredPage />);
    fireEvent.click(screen.getByRole('button', { name: 'Sign out instead' }));
    expect(logout).toHaveBeenCalledTimes(1);
  });
});
