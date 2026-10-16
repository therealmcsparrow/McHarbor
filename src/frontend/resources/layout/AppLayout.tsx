// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { Outlet, Navigate, useLocation, useMatches } from 'react-router';
import { PageShell, type ScrollMode } from './PageShell';
import { Aurora } from './Aurora';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { Footer } from './Footer';
import { useAuth } from '@core/auth/useAuth';
import { loginPathFor } from '@core/auth/returnTo';
import { useResolvedLayout } from '@resources/hooks/useLayoutMode';
import { useDockerEvents } from '@resources/hooks/useDockerEvents';
import { useDockerDiskUsageNotifications } from '@resources/hooks/useDockerDiskUsageNotifications';
import { useGlobalShortcuts } from '@resources/hooks/useShortcuts';
import { Spinner } from '@resources/components/ui/Spinner';
import { ShellSlot } from './shellSlots';
import { ShortcutsCheatsheet } from '@resources/components/ShortcutsCheatsheet';
import { WhatsNewOverlay } from '@resources/components/WhatsNewOverlay';
import { SecretInsertProvider } from '@resources/components/secrets/SecretInsertProvider';
import { PluginBridges } from '@core/plugins/PluginBridges';
import { pluginLoader } from '@core/plugins/loader';
import { AIAssistantDialog } from '@resources/components/AIAssistantDialog';
import { AIContextProvider } from '@resources/ai';
import { OPEN_AI_ASSISTANT_EVENT, type OpenAIAssistantDetail } from '@resources/ai/openAssistant';

declare global {
  interface Window {
    /** Lets plugin bundles retry the loader without importing the host. */
    mcLoaderInit?: typeof pluginLoader.init;
  }
}

export function AppLayout() {
  const { isAuthenticated, isLoading, needsSetup } = useAuth();
  const matches = useMatches();
  const loc = useLocation();
  const layout = useResolvedLayout();
  useDockerEvents();
  useDockerDiskUsageNotifications();

  const [cheatsheetOpen, setCheatsheetOpen] = useState(false);
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [assistantSeed, setAssistantSeed] = useState<string | null>(null);
  useGlobalShortcuts({
    onRefresh: () => window.location.reload(),
    onShowShortcuts: () => setCheatsheetOpen((v) => !v),
  });

  // Bootstrap the plugin loader exactly once after the user is
  // authenticated. The `idle` gate means re-renders (e.g. language
  // change) won't refire; the loader's `refresh({force:true})` path
  // handles installs/uninstalls. Exposed on `window` so the
  // status page (and the manual "Refresh plugins" button) can call
  // it explicitly.
  useEffect(() => {
    if (!isAuthenticated) return;
    if (pluginLoader.status() !== 'idle') return;
    void pluginLoader.init();
    window.mcLoaderInit = pluginLoader.init;
  }, [isAuthenticated]);

  useEffect(() => {
    function handleOpenAssistant(evt: Event) {
      const detail = (evt as CustomEvent<OpenAIAssistantDetail>).detail;
      setAssistantSeed(detail?.prompt ?? null);
      setAssistantOpen(true);
    }
    window.addEventListener(OPEN_AI_ASSISTANT_EVENT, handleOpenAssistant);
    return () => window.removeEventListener(OPEN_AI_ASSISTANT_EVENT, handleOpenAssistant);
  }, []);

  // Scroll behaviour is declared by the route via `handle.scroll` and read
  // here, rather than inferred from the pathname. The deepest match wins, so
  // a nested route can override its parent. Anything that forgets to declare
  // gets the fixed-frame default, which is what most pages want.
  //
  // This replaces a nine-prefix `pathname.startsWith(...)` chain that every
  // new route had to remember to update — see `PageShell` for the contract.
  const scrollMode: ScrollMode =
    (matches
      .map((m) => (m.handle as { scroll?: ScrollMode } | undefined)?.scroll)
      .filter(Boolean)
      .pop() as ScrollMode | undefined) ?? 'data';

  const padded =
    (matches
      .map((m) => (m.handle as { padded?: boolean } | undefined)?.padded)
      .filter((v) => v !== undefined)
      .pop() as boolean | undefined) ?? true;

  const sidebar =
    (matches
      .map((m) => (m.handle as { sidebar?: boolean } | undefined)?.sidebar)
      .filter((v) => v !== undefined)
      .pop() as boolean | undefined) ?? false;

  const onPhone = matches.some((m) => (m.handle as { onPhone?: boolean } | undefined)?.onPhone === true);

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
    return <Navigate to={loginPathFor(loc)} replace />;
  }

  if (layout === 'oncall' && !onPhone) {
    return <Navigate to="/oncall" replace />;
  }

  return (
    <AIContextProvider>
      {/* The aurora sits behind everything at z-0; the shell is z-10 glass
        over it. Without something drifting underneath, the glass panels
        have nothing to blur and read as flat low-contrast cards. */}
      <Aurora />
      <div className="relative z-10 flex h-screen overflow-hidden bg-transparent">
        <Sidebar />
        <div className="flex flex-1 flex-col overflow-hidden">
          <Header />
          <ShellSlot name="afterHeader" />
          <div id="pagehead-slot" className="shrink-0" />
          <ShellSlot name="banners" />
          {/* `main` is a frame and never scrolls; PageShell owns the one
            scroller on the screen. Padding lives on the shell so that on a
            flat canvas it scrolls with the content, as it did before. */}
          <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
            <PageShell scroll={scrollMode} padded={padded} sidebar={sidebar}>
              <Outlet />
            </PageShell>
          </main>
          <Footer />
        </div>
        <ShortcutsCheatsheet open={cheatsheetOpen} onOpenChange={setCheatsheetOpen} />
        <WhatsNewOverlay />
        <PluginBridges />
        {/* Right-click any text or number field to insert a `${secret:NAME}`
          reference. One document-level listener rather than a prop threaded
          through every form — see SecretInsertProvider for why, and for the
          Shift+right-click escape hatch back to the browser's own menu. */}
        <SecretInsertProvider />
        {/*
        The floating "Ask AI" sphere used to sit here, bottom-right, on
        every page. Removed: it overlapped page content, and it was the
        weaker of two triggers — the header's assistant button seeds the
        prompt with the focused entity, while the FAB opened a generic
        one. The assistant is still reachable from that header button and
        from the command palette (`shell:assistant:open`).
      */}
        <AIAssistantDialog
          open={assistantOpen}
          onOpenChange={setAssistantOpen}
          seedPrompt={assistantSeed}
          onSeedConsumed={() => setAssistantSeed(null)}
        />
      </div>
    </AIContextProvider>
  );
}
