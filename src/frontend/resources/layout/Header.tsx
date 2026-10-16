// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense } from 'react';
import {
  IconSearch,
  IconLayoutSidebarLeftCollapse,
  IconLayoutSidebarLeftExpand,
  IconSparkles,
} from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { useAuth } from '@core/auth/useAuth';
import { HeaderOverlays } from './HeaderOverlays';
import { HeaderUserMenu } from './HeaderUserMenu';
import { useHeaderEnvironments } from './useHeaderEnvironments';
import { useHeaderOverlays } from './useHeaderOverlays';
import { api, type ApiResponse } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import {
  SYSTEM_RESTART_PERMISSION,
  hasPermission,
  useCurrentUserPermissions,
  type SystemRestartResponse,
} from '@core/auth/useCurrentUserPermissions';
import { useCallback, useState } from 'react';
import { useSidebarStore } from '@resources/stores/sidebar';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { DemoModeHeaderIcon } from '@resources/components/demo/DemoModeHeaderIcon';
import { openAssistant } from '@resources/ai/openAssistant';
import { useShellCommands } from './useShellCommands';
import { useAIContexts } from '@resources/ai/registry';
import { contextSeedPrompt, hasSeedableContext } from '@resources/ai/contextPrompt';
import { assertRestartScheduled, runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';

const HeaderNotifications = lazy(() =>
  import('@resources/layout/HeaderNotifications').then((module) => ({
    default: module.HeaderNotifications,
  })),
);

export function Header() {
  const { t } = useTranslation('common');
  const { t: tAI } = useTranslation('ai');
  const user = useAuth((s) => s.user);
  const logout = useAuth((s) => s.logout);
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toggleSidebar = useSidebarStore((s) => s.toggle);

  useHeaderEnvironments();

  // The page with the highest-priority registered AI context wins — the
  // container detail page outranks the shell.
  const { focused: focusedAI } = useAIContexts();
  const seedable = hasSeedableContext(focusedAI);
  const assistantLabel = seedable
    ? tAI('aiAssistant.askAbout', {
        name: focusedAI.resourceName ?? focusedAI.resourceId ?? '',
      })
    : tAI('aiAssistant.menuItem');
  const overlays = useHeaderOverlays();
  const [menuOpen, setMenuOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const userPermissions = useCurrentUserPermissions(!!user);
  const canRestartMcHarbor = hasPermission(userPermissions.data, SYSTEM_RESTART_PERMISSION);

  const restartMcHarbor = useMutation({
    mutationFn: () =>
      runWithMcHarborRestart(() =>
        api
          .post<SystemRestartResponse>('/system/restart')
          .then((res: ApiResponse<SystemRestartResponse>) =>
            assertRestartScheduled(res, t('system.restartDialog.notSupervised')),
          ),
      ),
    meta: { error: t('system.toast.restartFailed') },
    onSuccess: () => {
      setRestartOpen(false);
      setMenuOpen(false);
    },
  });

  const openRestart = useCallback(() => setRestartOpen(true), []);
  useShellCommands({
    canRestartMcHarbor,
    logout,
    onRestart: openRestart,
  });

  return (
    <>
      {/* z-30, not z-10: this element creates the stacking context the user
          menu lives in, and `<main>` comes after it in DOM order. At an equal
          z-10 the later sibling wins, so a sticky `<thead>` (z-10) painted over
          the open menu no matter how high the menu's own z-index went. Stays
          below the z-50 portalled overlays, which sit at body level. */}
      <header className="relative z-30 flex h-14 items-center border-b border-border bg-card px-4">
        {/* Left: sidebar toggle + page slot */}
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label={collapsed ? t('nav.expandSidebar') : t('nav.collapseSidebar')}
                onClick={toggleSidebar}
                className="size-8 shrink-0"
              >
                {collapsed ? (
                  <IconLayoutSidebarLeftExpand className="size-4.5" />
                ) : (
                  <IconLayoutSidebarLeftCollapse className="size-4.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{collapsed ? t('nav.expandSidebar') : t('nav.collapseSidebar')}</TooltipContent>
          </Tooltip>
          <div id="header-slot" className="flex min-w-0 flex-1 items-center" />
        </div>

        {/* Right side â€” always visible */}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {/* Page-specific right-side slot (e.g. view toggles) */}
          <div id="header-right-slot" className="flex items-center" />

          <DemoModeHeaderIcon />

          {/* Divider */}
          <div className="h-6 w-px bg-border" />

          {/* Context-seeded, unlike the floating AssistantFAB which opens
              blank. On a container detail page this arrives pre-loaded with
              that container's id, image and state, so the conversation does
              not start by establishing what we are looking at. */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                aria-label={assistantLabel}
                permission={PERM.AI_ASSISTANT_USE}
                onClick={() => openAssistant(seedable ? contextSeedPrompt(focusedAI, tAI) : undefined)}
                className="gap-2"
                data-testid="header-ai-chat"
              >
                <IconSparkles className="size-4 text-primary" />
                <span className="hidden xl:inline">{assistantLabel}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{assistantLabel}</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('header.search')}
                onClick={() => overlays.setSearchOpen(true)}
              >
                <IconSearch className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('header.search')}</TooltipContent>
          </Tooltip>

          <Suspense fallback={<div className="size-8" />}>
            <HeaderNotifications />
          </Suspense>

          {/* Divider */}
          <div className="h-6 w-px bg-border" />

          {user && (
            <HeaderUserMenu
              username={user.username}
              menuOpen={menuOpen}
              onMenuOpenChange={setMenuOpen}
              canRestartMcHarbor={canRestartMcHarbor}
              onRestart={openRestart}
              onLogout={logout}
            />
          )}
        </div>
      </header>

      <ConfirmDialog
        open={restartOpen}
        onOpenChange={(isOpen) => {
          setRestartOpen(isOpen);
          if (!isOpen && restartMcHarbor.isPending) {
            // Keep the spinner visible if a restart is already in flight.
            return;
          }
        }}
        title={t('system.restartDialog.title')}
        description={t('system.restartDialog.description')}
        confirmLabel={t('system.restartDialog.confirm')}
        variant="destructive"
        loading={restartMcHarbor.isPending}
        onConfirm={() => {
          restartMcHarbor.mutate();
        }}
      />

      <HeaderOverlays overlays={overlays} />
    </>
  );
}
