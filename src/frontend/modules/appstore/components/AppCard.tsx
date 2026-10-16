// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconEye } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { AppInstallationsSummary } from './AppInstallations';
import { AppLogo } from './AppLogo';
import type { AppTemplate } from '../types';
import { PERM } from '@core/auth/permissionKeys';

interface AppCardProps {
  app: AppTemplate;
  onInstall: (app: AppTemplate) => void;
  onViewDetail: (app: AppTemplate) => void;
}

function parseImageMeta(image: string) {
  const withoutDigest = image.split('@')[0] ?? image;
  const lastSlash = withoutDigest.lastIndexOf('/');
  const lastColon = withoutDigest.lastIndexOf(':');
  const path = lastColon > lastSlash ? withoutDigest.slice(0, lastColon) : withoutDigest;
  const parts = path.split('/').filter(Boolean);
  const hasRegistry = parts[0]?.includes('.') || parts[0]?.includes(':');
  const registry = hasRegistry ? parts[0] : 'docker.io';
  const owner = hasRegistry ? parts[1] : parts.length > 1 ? parts[0] : 'library';

  return {
    owner: owner || 'library',
    distro: registry || 'docker.io',
  };
}

function formatDate(value: string, fallback: string) {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(date);
}

export function AppCard({ app, onInstall, onViewDetail }: AppCardProps) {
  const { t } = useTranslation('common');
  const imageMeta = parseImageMeta(app.image);
  // The catalog server states both facts outright in the app's `latest` block:
  // the real release version and the date it shipped. The registry lookup is
  // the fallback for a catalog that does not emit the block, and the catalog's
  // own version and sync date the fallback beneath that.
  const latestVersion = app.latest?.version || app.registryVersion || app.version;
  const updatedAt = app.latest?.release_date || app.registryUpdatedAt || app.updatedAt;

  return (
    <div className="group mc-glass flex h-full flex-col rounded-lg transition-colors hover:border-primary/40">
      <div className="m-2 flex flex-1 flex-col gap-3">
        <div className="flex items-start gap-3">
          <AppLogo appName={app.name} logo={app.logo} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h3 className="truncate text-sm font-medium text-foreground">{app.name}</h3>
              {app.version && <span className="shrink-0 text-xs text-muted-foreground">{app.version}</span>}
            </div>
            <Badge variant="secondary" className="mt-1 text-xs">
              {app.category}
            </Badge>
          </div>
        </div>

        <p className="line-clamp-2 flex-1 text-xs text-muted-foreground">{app.description}</p>

        <div className="truncate font-mono text-xs text-muted-foreground">{app.image}</div>

        <AppInstallationsSummary installations={app.installations ?? []} appName={app.name} />

        <div className="mt-auto flex flex-col gap-2">
          <div className="grid grid-cols-2 gap-x-3 gap-y-1 border-t border-border/60 pt-2 text-[11px] text-muted-foreground">
            <div className="min-w-0 truncate">
              <span className="text-foreground">{t('appStore.latestVersion')}:</span>{' '}
              {latestVersion || t('appStore.unknown')}
            </div>
            <div className="min-w-0 truncate text-right">
              <span className="text-foreground">{t('appStore.updated')}:</span>{' '}
              {formatDate(updatedAt, t('appStore.unknown'))}
            </div>
            <div className="min-w-0 truncate">
              <span className="text-foreground">{t('appStore.owner')}:</span> {imageMeta.owner}
            </div>
            <div className="min-w-0 truncate text-right">
              <span className="text-foreground">{t('appStore.distro')}:</span> {imageMeta.distro}
            </div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <Button size="sm" variant="ghost" className="justify-start gap-1 px-2" onClick={() => onViewDetail(app)}>
              <IconEye className="size-3.5" />
              {t('actions.review')}
            </Button>
            <Button
              permission={PERM.STORE_APPS_MANAGE}
              size="sm"
              variant="outline"
              className="justify-end gap-1"
              onClick={() => onInstall(app)}
            >
              <IconDownload className="size-3.5" />
              {t('appStore.install')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
