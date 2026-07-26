// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { Outlet, Navigate, useLocation } from 'react-router';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { Footer } from './Footer';
import { useAuth } from '@core/auth/useAuth';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useDockerEvents } from '@resources/hooks/useDockerEvents';
import { useDockerDiskUsageNotifications } from '@resources/hooks/useDockerDiskUsageNotifications';
import { useGlobalShortcuts } from '@resources/hooks/useShortcuts';
import { Spinner } from '@resources/components/ui/Spinner';
import { UpdateCheckBanner } from '@resources/components/UpdateCheckBanner';
import { FirstRunWizard } from '@resources/components/FirstRunWizard';
import { ShortcutsCheatsheet } from '@resources/components/ShortcutsCheatsheet';
import { WhatsNewOverlay } from '@resources/components/WhatsNewOverlay';

const APP_VERSION = '2.0.7';

export function AppLayout() {
  const { isAuthenticated, isLoading, needsSetup } = useAuth();
  const slotActive = useHeaderSlot((s) => s.active);
  const { pathname } = useLocation();
  useDockerEvents();
  useDockerDiskUsageNotifications();

  const [cheatsheetOpen, setCheatsheetOpen] = useState(false);
  useGlobalShortcuts({
    onRefresh: () => window.location.reload(),
    onShowShortcuts: () => setCheatsheetOpen((v) => !v),
  });

  // Pages render a flex-col layout that owns its own scrolling so
  // the table area (not the whole page) scrolls when the row
  // count exceeds the viewport. Detail / settings / security pages
  // need a flat canvas instead — they keep `overflow-visible` and
  // let the page itself scroll.
  const isFlatCanvas =
    pathname === '/store' ||
    pathname.startsWith('/settings') ||
    pathname === '/security' ||
    pathname.startsWith('/security/') ||
    pathname.startsWith('/containers/') ||
    pathname.startsWith('/images/') ||
    pathname.startsWith('/networks/') ||
    pathname.startsWith('/stacks/') ||
    pathname.startsWith('/environments/');

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
        <UpdateCheckBanner />
        <main
          className={`flex min-h-0 flex-1 flex-col ${
            isFlatCanvas ? 'overflow-y-auto' : 'overflow-hidden'
          } ${slotActive ? '' : 'p-6'}`}
        >
          <Outlet />
        </main>
        <Footer />
      </div>
      <FirstRunWizard />
      <ShortcutsCheatsheet open={cheatsheetOpen} onOpenChange={setCheatsheetOpen} />
      <WhatsNewOverlay version={APP_VERSION} />
    </div>
  );
}
