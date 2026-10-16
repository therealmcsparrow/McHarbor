// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPackages } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { HostUpdatesPanel } from '@resources/components/host/HostUpdatesPanel';
import type { HostTarget } from '@resources/hooks/useHostUpdates';

const SELF_HOST: HostTarget = { kind: 'self' };

export function HostUpdatesTab() {
  const { t } = useTranslation('settings');

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-border p-5">
        <div className="mb-4 flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <IconPackages className="size-4" />
          </span>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-foreground">{t('hostUpdates.title')}</h3>
            <p className="text-xs text-muted-foreground">{t('hostUpdates.selfSubtitle')}</p>
          </div>
        </div>

        <Alert variant="warning" title={t('hostUpdates.warningTitle')}>
          {t('hostUpdates.warningBody')}
        </Alert>
      </div>

      <HostUpdatesPanel target={SELF_HOST} />
    </div>
  );
}
