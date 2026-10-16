// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthLayout } from './AuthLayout';
import { PageHeader } from './PageHeader';

const authState = vi.hoisted(() => ({
  value: { isAuthenticated: false, isLoading: false },
}));

vi.mock('@core/auth/useAuth', () => ({
  useAuth: () => authState.value,
}));

vi.mock('react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-router')>()),
  Navigate: ({ to }: { to: string }) => <div data-testid="navigate">{to}</div>,
  Outlet: () => <div data-testid="outlet">Outlet</div>,
  useLocation: () => ({ pathname: '/login', search: '', hash: '', state: null, key: 'test' }),
}));

describe('AuthLayout', () => {
  it('shows a spinner while auth state loads', () => {
    authState.value = { isAuthenticated: false, isLoading: true };

    const { container } = render(<AuthLayout />);

    expect(container.firstChild).toHaveClass('h-screen');
    expect(screen.queryByTestId('outlet')).not.toBeInTheDocument();
  });

  it('redirects authenticated users and renders the outlet otherwise', () => {
    authState.value = { isAuthenticated: true, isLoading: false };
    const { rerender } = render(<AuthLayout />);

    expect(screen.getByTestId('navigate')).toHaveTextContent('/dashboard');

    authState.value = { isAuthenticated: false, isLoading: false };
    rerender(<AuthLayout />);

    expect(screen.getByTestId('outlet')).toBeInTheDocument();
  });
});

describe('PageHeader', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders into the header slot when available', async () => {
    const slot = document.createElement('div');
    slot.id = 'header-slot';
    document.body.appendChild(slot);
    const band = document.createElement('div');
    band.id = 'pagehead-slot';
    document.body.appendChild(band);

    render(<PageHeader title="Containers" description="Overview" actions={<button type="button">Refresh</button>} />);

    await waitFor(() => expect(screen.getByText('Containers')).toBeInTheDocument());
    expect(screen.getByRole('link', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByText('Overview')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument();
  });

  it('renders nothing when the header slot is absent', () => {
    document.getElementById('header-slot')?.remove();
    document.getElementById('pagehead-slot')?.remove();

    const { container } = render(<PageHeader title="Hidden" />);

    expect(container.firstChild).toBeNull();
  });
});
