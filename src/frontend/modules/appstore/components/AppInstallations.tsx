// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { AppInstallation } from '../types';

interface AppInstallationsSummaryProps {
  installations: AppInstallation[];
}

interface AppInstallationsListProps {
  installations: AppInstallation[];
}

function getEnvironmentLabel(installation: AppInstallation, fallback: string) {
  return installation.environmentName || installation.environmentId || fallback;
}

export function AppInstallationsSummary({ installations }: AppInstallationsSummaryProps) {
  const { t } = useTranslation('common');

  if (installations.length === 0) {
    return null;
  }

  return (
    <div className="rounded-md border border-border/60 bg-muted/30 px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('appStore.installedIn')}
        </span>
        <Badge variant="success">{installations.length}</Badge>
      </div>
      <p className="mt-2 line-clamp-2 text-xs text-foreground">
        {installations.map((installation) => getEnvironmentLabel(installation, t('appStore.unknownEnvironment'))).join(', ')}
      </p>
    </div>
  );
}

export function AppInstallationsList({ installations }: AppInstallationsListProps) {
  const { t } = useTranslation('common');

  if (installations.length === 0) {
    return null;
  }

  return (
    <div>
      <h4 className="mb-2 text-xs font-medium text-foreground">{t('appStore.installedIn')}</h4>
      <div className="space-y-2">
        {installations.map((installation) => (
          <div
            key={installation.id || installation.stackId}
            className="rounded-md border border-border/60 bg-muted/20 px-3 py-2"
          >
            <div className="flex items-center justify-between gap-3">
              <span className="truncate text-sm font-medium text-foreground">
                {getEnvironmentLabel(installation, t('appStore.unknownEnvironment'))}
              </span>
              <Badge variant="secondary" className="shrink-0">
                {installation.stackName}
              </Badge>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
