// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconBrandGithub, IconSparkles, IconWorld } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { api } from '@core/api/client';
import { SecurityPostureCard } from './SecurityPostureCard';
import { FactoryResetCard } from './FactoryResetCard';
import { frontendDeps } from './about-frontend-deps';
import packageJson from '../../../package.json';
import { assertSuccess } from '@resources/utils/api-mutation';
import { getReleaseItems } from '@resources/components/whatsNewItems';
import { useWhatsNewStore } from '@resources/stores/whats-new';

type AboutData = {
  version: string;
  build: string;
  goVersion: string;
  platform: string;
  dependencies: { name: string; version: string }[];
};

const websiteUrl = 'https://www.mcharbor.eu';
const repositoryUrl = 'https://github.com/therealmcsparrow/mcharbor';

function stripVersion(v: string): string {
  return v.replace(/^[\^~>=<]+/, '');
}

export function AboutTab() {
  const { t } = useTranslation('settings');

  const { data, isError, error, refetch } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<AboutData>('/about').then(assertSuccess),
    staleTime: 60_000,
  });

  const goVersion = data?.goVersion.replace('go', '') ?? '';
  const showWhatsNew = useWhatsNewStore((state) => state.show);
  const hasHighlights = data ? getReleaseItems(data.version).length > 0 : false;

  const allDeps = { ...packageJson.dependencies, ...packageJson.devDependencies } as Record<string, string>;

  return (
    <div className="space-y-8">
      {/* Branding header */}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-center gap-4">
          <img src="/logo_McHarbor.svg" alt={t('about.productName')} className="h-14" />
          <div>
            <h2 className="text-xl font-bold text-foreground">{t('about.productName')}</h2>
            <p className="text-sm text-muted-foreground">{t('about.tagline')}</p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <Button variant="outline" asChild>
            <a href={websiteUrl} target="_blank" rel="noreferrer">
              <IconWorld className="size-4" />
              {t('about.website')}
            </a>
          </Button>
          <Button variant="outline" onClick={showWhatsNew} disabled={!hasHighlights}>
            <IconSparkles className="size-4" />
            {t('about.whatsNew')}
          </Button>
          <Button variant="outline" asChild>
            <a href={repositoryUrl} target="_blank" rel="noreferrer">
              <IconBrandGithub className="size-4" />
              {t('about.repository')}
            </a>
          </Button>
          {data && (
            <>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium text-muted-foreground uppercase">{t('about.version')}</p>
                <p className="text-sm font-semibold text-foreground">
                  {t('about.versionValue', { version: data.version })}
                </p>
              </div>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium text-muted-foreground uppercase">{t('about.build')}</p>
                <p className="font-mono text-sm font-semibold text-foreground">{data.build}</p>
              </div>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium text-muted-foreground uppercase">{t('about.goLabel')}</p>
                <p className="text-sm font-semibold text-foreground">{goVersion}</p>
              </div>
              <div className="rounded-lg bg-muted px-3 py-1.5 text-center">
                <p className="text-[10px] font-medium text-muted-foreground uppercase">{t('about.platform')}</p>
                <p className="text-sm font-semibold text-foreground">{data.platform}</p>
              </div>
            </>
          )}
        </div>
      </div>

      {isError && <ErrorState variant="banner" error={error} onRetry={() => void refetch()} />}

      <SecurityPostureCard />

      {/* Danger Zone — factory reset */}
      <FactoryResetCard />

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
        {t('about.copyrightHolder')} {t('about.allRightsReserved')}
      </div>
    </div>
  );
}
