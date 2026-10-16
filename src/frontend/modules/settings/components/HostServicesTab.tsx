// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconServerCog } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { HostServicesPanel } from '@resources/components/host/HostServicesPanel';
import type { HostTarget } from '@resources/hooks/useHostUpdates';

const SELF_HOST: HostTarget = { kind: 'self' };

export function HostServicesTab() {
  const { t } = useTranslation('settings');

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-6">
      <div className="rounded-lg border border-border p-5">
        <div className="mb-4 flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <IconServerCog className="size-4" />
          </span>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-foreground">{t('hostServices.title')}</h3>
            <p className="text-xs text-muted-foreground">{t('hostServices.selfSubtitle')}</p>
          </div>
        </div>

        <Alert variant="warning" title={t('hostServices.warningTitle')}>
          {t('hostServices.warningBody')}
        </Alert>
      </div>

      <HostServicesPanel host={SELF_HOST} tableId="settings-host-services" />
    </div>
  );
}
