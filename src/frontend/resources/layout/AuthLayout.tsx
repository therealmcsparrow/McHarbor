// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Outlet, Navigate, useLocation } from 'react-router';
import { useAuth } from '@core/auth/useAuth';
import { Spinner } from '@resources/components/ui/Spinner';

export function AuthLayout() {
  const { isAuthenticated, isLoading, needsSetup } = useAuth();
  const { pathname } = useLocation();

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <Spinner size="xl" />
      </div>
    );
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />;
  }

  // A fresh database has no users, so every public page must funnel
  // into first-run setup. Only `/` did this before, which left a
  // browser that reloaded on `/login` stuck there after a reset.
  const onSetup = pathname === '/setup' || pathname.startsWith('/setup/');
  if (needsSetup && !onSetup) {
    return <Navigate to="/setup" replace />;
  }
  if (!needsSetup && onSetup) {
    return <Navigate to="/login" replace />;
  }

  return <Outlet />;
}
