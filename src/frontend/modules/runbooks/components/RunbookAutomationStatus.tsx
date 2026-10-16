// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import { describeCron } from '@resources/utils/schedule';
import { formatDate } from '@resources/utils/format';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import type { RunbookAutomation } from '@resources/types/runbooks';

type RunbookAutomationStatusProps = {
  automation: RunbookAutomation;
};

export function RunbookAutomationStatus({ automation }: RunbookAutomationStatusProps) {
  const { t } = useTranslation('runbooks');
  const { t: tc } = useTranslation('common');
  const formatPrefs = useLocaleFormat();
  const active = Boolean(automation.principalId) && (automation.onAlert || Boolean(automation.cron));

  if (!active) {
    return <p className="text-sm text-muted-foreground">{t('automation.off')}</p>;
  }

  return (
    <dl className="grid gap-1 text-sm text-foreground sm:grid-cols-[max-content_1fr] sm:gap-x-4">
      <dt className="text-muted-foreground">{t('automation.runsAs')}</dt>
      <dd className="flex items-center gap-2">
        <span className="font-medium">{automation.principalName || automation.principalId}</span>
        <Badge variant="secondary">{t('automation.servicePrincipal')}</Badge>
      </dd>
      {automation.onAlert && (
        <>
          <dt className="text-muted-foreground">{t('automation.onAlert')}</dt>
          <dd>{t('automation.onAlertActive')}</dd>
        </>
      )}
      {automation.cron && (
        <>
          <dt className="text-muted-foreground">{t('automation.schedule')}</dt>
          <dd>
            {describeCron(automation.cron, tc)}
            <span className="ml-2 font-mono opacity-70">{automation.cron}</span>
          </dd>
          <dt className="text-muted-foreground">{t('automation.nextRun')}</dt>
          <dd>{formatDate(automation.nextRunAt, formatPrefs)}</dd>
        </>
      )}
      <dt className="text-muted-foreground">{t('automation.lastRun')}</dt>
      <dd className="flex items-center gap-2">
        {automation.lastRunAt ? formatDate(automation.lastRunAt, formatPrefs) : t('automation.never')}
        <StatusError error={automation.lastError} title={t('automation.lastErrorTitle')} />
      </dd>
    </dl>
  );
}
