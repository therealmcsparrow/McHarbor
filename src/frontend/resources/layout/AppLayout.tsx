// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Outlet, Navigate, useLocation } from 'react-router';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { Footer } from './Footer';
import { useAuth } from '@core/auth/useAuth';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useDockerEvents } from '@resources/hooks/useDockerEvents';
import { useDockerDiskUsageNotifications } from '@resources/hooks/useDockerDiskUsageNotifications';
import { Spinner } from '@resources/components/ui/Spinner';

export function AppLayout() {
  const { isAuthenticated, isLoading, needsSetup } = useAuth();
  const slotActive = useHeaderSlot((s) => s.active);
  const { pathname } = useLocation();
  useDockerEvents();
  useDockerDiskUsageNotifications();

  const routeOwnsScroll = pathname === '/store';

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <Spinner size="xl" />
      </div>
    );
  }

  if (needsSetup) {
    return <Navigate to="/setup" replace />;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Header />
        <main className={`min-h-0 flex-1 ${routeOwnsScroll ? 'overflow-hidden' : 'overflow-y-auto'} ${slotActive ? '' : 'p-6'}`}>
          <Outlet />
        </main>
        <Footer />
      </div>
    </div>
  );
}
