// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconRefresh, IconArrowUp, IconBrandGithub, IconCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { api } from '@core/api/client';
import { useCheckUpdate } from '../hooks/useUpdates';
import packageJson from '../../../package.json';

type AboutData = {
  version: string;
  goVersion: string;
  platform: string;
  dependencies: { name: string; version: string }[];
};

const repositoryUrl = 'https://github.com/therealmcsparrow/mcharbor';

const frontendDeps: { name: string; label: string }[] = [
  { name: 'react', label: 'React' },
  { name: 'react-dom', label: 'React DOM' },
  { name: 'react-router', label: 'React Router' },
  { name: 'vite', label: 'Vite' },
  { name: 'typescript', label: 'TypeScript' },
  { name: '@tanstack/react-query', label: 'TanStack Query' },
  { name: '@tanstack/react-table', label: 'TanStack Table' },
  { name: 'tailwindcss', label: 'Tailwind CSS' },
  { name: 'zustand', label: 'Zustand' },
  { name: '@radix-ui/react-dialog', label: 'Radix UI Dialog' },
  { name: '@radix-ui/react-dropdown-menu', label: 'Radix UI Dropdown' },
  { name: '@radix-ui/react-select', label: 'Radix UI Select' },
  { name: '@radix-ui/react-tabs', label: 'Radix UI Tabs' },
  { name: '@radix-ui/react-tooltip', label: 'Radix UI Tooltip' },
  { name: '@radix-ui/react-popover', label: 'Radix UI Popover' },
  { name: '@radix-ui/react-switch', label: 'Radix UI Switch' },
  { name: '@radix-ui/react-checkbox', label: 'Radix UI Checkbox' },
  { name: '@radix-ui/react-label', label: 'Radix UI Label' },
  { name: '@radix-ui/react-separator', label: 'Radix UI Separator' },
  { name: '@radix-ui/react-scroll-area', label: 'Radix UI Scroll Area' },
  { name: '@radix-ui/react-slot', label: 'Radix UI Slot' },
  { name: '@tabler/icons-react', label: 'Tabler Icons' },
  { name: '@tailwindcss/forms', label: 'Tailwind Forms' },
  { name: 'react-i18next', label: 'react-i18next' },
  { name: 'i18next', label: 'i18next' },
  { name: 'i18next-browser-languagedetector', label: 'i18next Language Detector' },
  { name: 'recharts', label: 'Recharts' },
  { name: 'react-grid-layout', label: 'React Grid Layout' },
  { name: 'react-hook-form', label: 'React Hook Form' },
  { name: '@codemirror/view', label: 'CodeMirror View' },
  { name: '@codemirror/lang-javascript', label: 'CodeMirror JS' },
  { name: '@codemirror/lang-yaml', label: 'CodeMirror YAML' },
  { name: '@xterm/xterm', label: 'xterm.js' },
  { name: '@xterm/addon-fit', label: 'xterm Fit' },
  { name: '@xterm/addon-web-links', label: 'xterm Web Links' },
  { name: 'sonner', label: 'Sonner' },
  { name: 'cmdk', label: 'cmdk' },
  { name: 'zod', label: 'Zod' },
  { name: 'class-variance-authority', label: 'CVA' },
  { name: 'clsx', label: 'clsx' },
  { name: 'tailwind-merge', label: 'tailwind-merge' },
  { name: 'eslint', label: 'ESLint' },
  { name: 'prettier', label: 'Prettier' },
  { name: '@vitejs/plugin-react', label: 'Vite React Plugin' },
  { name: 'autoprefixer', label: 'Autoprefixer' },
];

function stripVersion(v: string): string {
  return v.replace(/^[\^~>=<]+/, '');
}

export function AboutTab() {
  const { t } = useTranslation('settings');

  const { data } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<AboutData>('/about').then((r) => r.data),
    staleTime: 60_000,
  });

  const {
    data: versionCheck,
    refetch: checkUpdate,
    isError: checkFailed,
    isFetching: isChecking,
  } = useCheckUpdate();
  const allDeps = { ...packageJson.dependencies, ...packageJson.devDependencies } as Record<string, string>;

  return (
    <div className="space-y-8">
      {/* Branding header */}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-center gap-4">
          <img src="/logo_McHarbor.svg" alt="McHarbor" className="h-14" />
          <div>
            <h2 className="text-xl font-bold text-foreground">McHarbor</h2>
            <p className="text-sm text-muted-foreground">
              {t('about.tagline')}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <Button variant="outline" asChild>
            <a href={repositoryUrl} target="_blank" rel="noreferrer">
              <IconBrandGithub className="size-4" />
              {t('about.repository')}
            </a>
          </Button>
          {data && (
            <>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium uppercase text-muted-foreground">{t('about.version')}</p>
                <p className="text-sm font-semibold text-foreground">v{data.version}</p>
              </div>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium uppercase text-muted-foreground">Go</p>
                <p className="text-sm font-semibold text-foreground">{data.goVersion.replace('go', '')}</p>
              </div>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium uppercase text-muted-foreground">{t('about.platform')}</p>
                <p className="text-sm font-semibold text-foreground">{data.platform}</p>
              </div>
            </>
          )}
        </div>
      </div>

      {/* Check for Updates */}
      <div className="rounded-lg border border-border p-5">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-foreground">{t('about.checkForUpdates')}</h3>
            <p className="text-sm text-muted-foreground">{t('about.checkForUpdatesDescription')}</p>
          </div>
          <Button
            variant="outline"
            onClick={() => checkUpdate()}
            disabled={isChecking}
          >
            {isChecking ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
            {t('about.checkNow')}
          </Button>
        </div>

        {checkFailed && (
          <div className="mt-4 rounded-lg bg-destructive/10 p-4 text-sm text-destructive">
            {t('about.checkFailed')}
          </div>
        )}

        {!checkFailed && versionCheck && (
          <div className="mt-4 rounded-lg bg-muted p-4">
            {versionCheck.updateAvailable ? (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <IconArrowUp className="size-4 text-emerald-400" />
                  <span className="text-sm font-medium text-foreground">
                    {t('about.updateAvailable', { version: versionCheck.latestVersion })}
                  </span>
                  <Badge variant="default">{t('about.newVersion')}</Badge>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('about.currentVersionLabel')}: v{versionCheck.currentVersion} &rarr; v{versionCheck.latestVersion}
                </p>
                {versionCheck.releaseNotes && (
                  <p className="max-h-32 overflow-y-auto whitespace-pre-wrap text-xs text-muted-foreground">
                    {versionCheck.releaseNotes}
                  </p>
                )}
                {versionCheck.releaseUrl && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.open(versionCheck.releaseUrl, '_blank', 'noopener')}
                  >
                    {t('about.viewRelease')}
                  </Button>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <IconCheck className="size-4 text-emerald-400" />
                <span className="text-sm text-foreground">{t('about.upToDate')}</span>
                <span className="text-xs text-muted-foreground">v{versionCheck.currentVersion}</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Backend dependencies */}
      {data && data.dependencies.length > 0 && (
        <div>
          <h3 className="mb-3 text-sm font-semibold text-foreground">
            {t('about.backendDeps')}
            <span className="ml-2 text-xs font-normal text-muted-foreground">({data.dependencies.length})</span>
          </h3>
          <div className="rounded-lg border border-border">
            <div className="grid grid-cols-2 gap-px bg-border sm:grid-cols-3 lg:grid-cols-4">
              {data.dependencies.map((dep) => (
                <div key={dep.name} className="flex items-center justify-between bg-card px-3 py-2">
                  <span className="text-sm text-foreground">{dep.name}</span>
                  <span className="ml-2 shrink-0 font-mono text-xs text-muted-foreground">{dep.version}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Frontend dependencies */}
      <div>
        <h3 className="mb-3 text-sm font-semibold text-foreground">
          {t('about.frontendDeps')}
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            ({frontendDeps.filter((d) => allDeps[d.name]).length})
          </span>
        </h3>
        <div className="rounded-lg border border-border">
          <div className="grid grid-cols-2 gap-px bg-border sm:grid-cols-3 lg:grid-cols-4">
            {frontendDeps.map((dep) => {
              const ver = allDeps[dep.name];
              if (!ver) return null;
              return (
                <div key={dep.name} className="flex items-center justify-between bg-card px-3 py-2">
                  <span className="text-sm text-foreground">{dep.label}</span>
                  <span className="ml-2 shrink-0 font-mono text-xs text-muted-foreground">{stripVersion(ver)}</span>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* Footer */}
      <div className="border-t border-border pt-4 text-sm text-muted-foreground">
        © 2026 McSparrow. {t('about.allRightsReserved')}
      </div>
    </div>
  );
}
