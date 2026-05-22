// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense } from 'react';
import { IconMoon, IconSun, IconDeviceDesktop, IconLogout, IconSearch, IconCheck, IconLanguage, IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand, IconInfoCircle } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@resources/hooks/useTheme';
import { useAuth } from '@core/auth/useAuth';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useLanguageStore } from '@resources/stores/language';
import { supportedLanguages, languageLabels, type SupportedLanguage } from '@core/i18n';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useState, useRef, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useSidebarStore } from '@resources/stores/sidebar';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';

const GlobalSearch = lazy(() => import('@resources/components/GlobalSearch').then((module) => ({ default: module.GlobalSearch })));
const ContainerOverview = lazy(() => import('@resources/components/ContainerOverview').then((module) => ({ default: module.ContainerOverview })));
const ImageOverview = lazy(() => import('@resources/components/ImageOverview').then((module) => ({ default: module.ImageOverview })));
const VolumeOverview = lazy(() => import('@resources/components/VolumeOverview').then((module) => ({ default: module.VolumeOverview })));
const StackOverview = lazy(() => import('@resources/components/StackOverview').then((module) => ({ default: module.StackOverview })));
const HeaderNotifications = lazy(() => import('@resources/layout/HeaderNotifications').then((module) => ({ default: module.HeaderNotifications })));

const ENV_SELECTOR_ROUTES = new Set([
  '/',
  '/containers',
  '/images',
  '/volumes',
  '/networks',
  '/stacks',
  '/terminal',
  '/logs',
  '/pods',
  '/deployments',
  '/k8s-services',
  '/namespaces',
  '/activity',
  '/audit',
  '/app-store',
  '/docker',
]);

export function Header() {
  const { t } = useTranslation('common');
  const { theme, setTheme } = useTheme();
  const user = useAuth((s) => s.user);
  const logout = useAuth((s) => s.logout);
  const environments = useEnvironmentStore((s) => s.environments);
  const currentId = useEnvironmentStore((s) => s.currentId);
  const setCurrentId = useEnvironmentStore((s) => s.setCurrentId);
  const setEnvironments = useEnvironmentStore((s) => s.setEnvironments);
  const language = useLanguageStore((s) => s.language);
  const setLanguage = useLanguageStore((s) => s.setLanguage);
  const { pathname } = useLocation();
  const showEnvSelector = ENV_SELECTOR_ROUTES.has(pathname);
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toggleSidebar = useSidebarStore((s) => s.toggle);

  const themeOptions = [
    { value: 'light' as const, label: t('theme.light'), icon: IconSun },
    { value: 'dark' as const, label: t('theme.dark'), icon: IconMoon },
    { value: 'system' as const, label: t('theme.system'), icon: IconDeviceDesktop },
  ];

  type EnvSummary = {
    id: string;
    name: string;
    orchestratorType: 'docker' | 'kubernetes';
    connectionType: string;
    isDefault: boolean;
    dockerVersion: string | null;
    k8sVersion: string | null;
    scheduledUpdateCheckEnabled: boolean;
    automaticImagePruningEnabled: boolean;
    trackContainerEventsEnabled: boolean;
    collectContainerMetricsEnabled: boolean;
    highlightContainerChangesEnabled: boolean;
    dockerDiskUsageNotificationsEnabled: boolean;
    dockerDiskUsageThresholdPercent: number;
  };
  const environmentsQuery = useQuery({
    queryKey: ['environments'],
    queryFn: () => api.get<EnvSummary[]>('/environments').then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });

  const navigate = useNavigate();
  const [searchOpen, setSearchOpen] = useState(false);
  const [containerOverviewOpen, setContainerOverviewOpen] = useState(false);
  const [imageOverviewOpen, setImageOverviewOpen] = useState(false);
  const [volumeOverviewOpen, setVolumeOverviewOpen] = useState(false);
  const [stackOverviewOpen, setStackOverviewOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (environmentsQuery.data) {
      setEnvironments(environmentsQuery.data);
    }
  }, [environmentsQuery.data, setEnvironments]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (e.target as HTMLElement)?.isContentEditable) return;

      switch (e.key) {
        case 'c':
          e.preventDefault();
          setContainerOverviewOpen((prev) => !prev);
          break;
        case 'i':
          e.preventDefault();
          setImageOverviewOpen((prev) => !prev);
          break;
        case 'v':
          e.preventDefault();
          setVolumeOverviewOpen((prev) => !prev);
          break;
        case 's':
          e.preventDefault();
          setStackOverviewOpen((prev) => !prev);
          break;
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    if (menuOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [menuOpen]);

  return (
    <>
      <header className="relative flex h-14 items-center border-b border-border bg-card px-4">
        {/* Left: sidebar toggle + environment selector + page slot */}
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

          {showEnvSelector && environments.length > 0 && (
            <>
              <select
                value={currentId}
                onChange={(e) => setCurrentId(e.target.value)}
                className="shrink-0 py-2 px-3 pe-9 bg-card border border-border rounded-lg text-sm text-foreground focus:border-primary focus:ring-primary"
              >
                <option value="">{t('header.allEnvironments')}</option>
                {environments.map((env) => (
                  <option key={env.id} value={env.id}>
                    {env.name}
                  </option>
                ))}
              </select>
              <div className="h-6 w-px bg-border shrink-0" />
            </>
          )}
          <div id="header-slot" className="flex min-w-0 flex-1 items-center" />
        </div>

        {/* Center: search bar — absolutely centered */}
        <div className="absolute left-1/2 top-1/2 hidden -translate-x-1/2 -translate-y-1/2 md:flex">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setSearchOpen(true)}
            className="text-muted-foreground"
          >
            <IconSearch className="size-3.5" />
            <span>{t('header.search')}</span>
          </Button>
        </div>

        {/* Right side — always visible */}
        <div className="flex shrink-0 items-center gap-2 ml-auto">
          {/* Page-specific right-side slot (e.g. view toggles) */}
          <div id="header-right-slot" className="flex items-center" />

          {/* Divider */}
          <div className="h-6 w-px bg-border" />

          <Suspense fallback={<div className="size-8" />}>
            <HeaderNotifications />
          </Suspense>

          {/* Divider */}
          <div className="h-6 w-px bg-border" />

          {/* User avatar + dropdown */}
          {user && (
            <div className="relative" ref={menuRef}>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setMenuOpen(!menuOpen)}
                aria-label={t('header.userMenu')}
              >
                <div className="flex size-7 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
                  {user.username.charAt(0).toUpperCase()}
                </div>
              </Button>

              {menuOpen && (
                <div className="absolute right-0 top-full z-50 mt-1 w-52 bg-card border border-border rounded-lg shadow-lg py-1">
                  {/* User info */}
                  <div className="px-3 py-2.5 border-b border-border">
                    <div className="flex items-center gap-2.5">
                      <div className="flex size-8 items-center justify-center rounded-full bg-primary text-sm font-medium text-primary-foreground">
                        {user.username.charAt(0).toUpperCase()}
                      </div>
                      <div>
                        <p className="text-sm font-medium text-foreground">{user.username}</p>
                        <p className="text-xs text-muted-foreground">{t('header.administrator')}</p>
                      </div>
                    </div>
                  </div>

                  {/* Theme section */}
                  <div className="px-3 py-1.5 text-xs font-medium uppercase text-muted-foreground">{t('theme.label')}</div>
                  {themeOptions.map((opt) => (
                    <Button
                      key={opt.value}
                      type="button"
                      variant="ghost"
                      onClick={() => setTheme(opt.value)}
                      className="w-full justify-start rounded-none px-3 py-1.5 text-sm"
                    >
                      <opt.icon className="size-4 text-muted-foreground" />
                      <span>{opt.label}</span>
                      {theme === opt.value && <IconCheck className="ml-auto size-3.5 text-primary" />}
                    </Button>
                  ))}

                  {/* Language section */}
                  <div className="my-1 border-t border-border" />
                  <div className="px-3 py-1.5 text-xs font-medium uppercase text-muted-foreground">{t('language.label')}</div>
                  {supportedLanguages.map((lang) => (
                    <Button
                      key={lang}
                      type="button"
                      variant="ghost"
                      onClick={() => setLanguage(lang as SupportedLanguage)}
                      className="w-full justify-start rounded-none px-3 py-1.5 text-sm"
                    >
                      <IconLanguage className="size-4 text-muted-foreground" />
                      <span>{languageLabels[lang]}</span>
                      {language === lang && <IconCheck className="ml-auto size-3.5 text-primary" />}
                    </Button>
                  ))}

                  <div className="my-1 border-t border-border" />
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => { setMenuOpen(false); navigate('/settings?tab=about'); }}
                    className="w-full justify-start rounded-none px-3 py-1.5 text-sm"
                  >
                    <IconInfoCircle className="size-4 text-muted-foreground" />
                    <span>{t('about.menuItem')}</span>
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => { setMenuOpen(false); logout(); }}
                    className="w-full justify-start rounded-none px-3 py-1.5 text-sm text-destructive hover:text-destructive"
                  >
                    <IconLogout className="size-4" />
                    <span>{t('actions.signOut')}</span>
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      </header>

      {searchOpen && (
        <Suspense fallback={null}>
          <GlobalSearch open={searchOpen} onOpenChange={setSearchOpen} />
        </Suspense>
      )}
      {containerOverviewOpen && (
        <Suspense fallback={null}>
          <ContainerOverview open={containerOverviewOpen} onOpenChange={setContainerOverviewOpen} />
        </Suspense>
      )}
      {imageOverviewOpen && (
        <Suspense fallback={null}>
          <ImageOverview open={imageOverviewOpen} onOpenChange={setImageOverviewOpen} />
        </Suspense>
      )}
      {volumeOverviewOpen && (
        <Suspense fallback={null}>
          <VolumeOverview open={volumeOverviewOpen} onOpenChange={setVolumeOverviewOpen} />
        </Suspense>
      )}
      {stackOverviewOpen && (
        <Suspense fallback={null}>
          <StackOverview open={stackOverviewOpen} onOpenChange={setStackOverviewOpen} />
        </Suspense>
      )}
    </>
  );
}

