// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Navigate, Outlet, useLocation } from 'react-router';
import { useAuth } from '@core/auth/useAuth';
import { loginPathFor } from '@core/auth/returnTo';
import { Spinner } from '@resources/components/ui/Spinner';
import { useBrandLogo } from '@resources/hooks/useBranding';
import { BottomBar } from '../components/BottomBar';

const BRAND_NAME = 'McHarbor';

export function OnCallLayout() {
  const { isAuthenticated, isLoading, needsSetup } = useAuth();
  const locationValue = useLocation();
  const logo = useBrandLogo('wide');

  if (isLoading) {
    return (
      <div className="flex h-dvh items-center justify-center bg-background">
        <Spinner size="xl" />
      </div>
    );
  }
  if (needsSetup) return <Navigate to="/setup" replace />;
  if (!isAuthenticated) return <Navigate to={loginPathFor(locationValue)} replace />;

  return (
    <div className="flex h-dvh flex-col bg-background text-foreground">
      <header className="flex h-12 shrink-0 items-center justify-center border-b border-border px-4 pt-[env(safe-area-inset-top)]">
        <img src={logo.src} alt={BRAND_NAME} className="h-6 w-auto" />
      </header>
      <main className="mx-auto flex min-h-0 w-full max-w-xl flex-1 flex-col overflow-y-auto px-4 py-4">
        <Outlet />
      </main>
      <BottomBar />
    </div>
  );
}
