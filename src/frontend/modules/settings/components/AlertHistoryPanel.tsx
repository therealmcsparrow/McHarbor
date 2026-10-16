// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconFlame, IconHistory, IconX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDate } from '@resources/utils/format';
import { useAlertEvents, type AlertEventState } from '@resources/hooks/useAlertEvents';
import { ALERT_SEVERITY_BADGE_VARIANTS, ALERT_SEVERITY_LABEL_KEYS } from './alert-options';

/**
 * What actually fired, and whether it reached anyone.
 *
 * History outlives the rules that produced it, so a row here can name
 * a rule that no longer exists — which is exactly the row you want
 * after deleting something noisy.
 */
export function AlertHistoryPanel() {
  const { t } = useTranslation('settings');
  const [state, setState] = useState<AlertEventState | ''>('');
  const { data, isLoading } = useAlertEvents({ state, perPage: 50 });

  const events = data?.items ?? [];

  const stateOptions = [
    { value: '', label: t('alerts.historyAllStates') },
    { value: 'firing', label: t('alerts.stateFiring') },
    { value: 'resolved', label: t('alerts.stateResolved') },
  ];

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="max-w-2xl text-sm text-muted-foreground">{t('alerts.historyDescription')}</p>
        <div className="w-56">
          <Select
            value={state}
            onChange={(value) => setState(value as AlertEventState | '')}
            options={stateOptions}
            searchable={false}
            ariaLabel={t('alerts.historyFilterLabel')}
          />
        </div>
      </div>

      {events.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
          <IconHistory className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">{t('alerts.historyEmpty')}</p>
          <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('alerts.historyEmptyDescription')}</p>
        </div>
      ) : (
        <div className="divide-y divide-border rounded-lg border border-border">
          {events.map((alertEvent) => {
            const failed = alertEvent.deliveries.filter((delivery) => !delivery.ok).length;
            const delivered = alertEvent.deliveries.length - failed;

            return (
              <div key={alertEvent.id} className="flex flex-col gap-2 p-4 sm:flex-row sm:items-start sm:gap-4">
                <div className="flex w-40 shrink-0 items-center gap-2">
                  {alertEvent.state === 'firing' ? (
                    <IconFlame className="size-4 shrink-0 text-destructive" />
                  ) : (
                    <IconCheck className="size-4 shrink-0 text-success" />
                  )}
                  <span className="truncate text-xs text-muted-foreground">{formatDate(alertEvent.createdAt)}</span>
                </div>

                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">{alertEvent.alertName}</span>
                    <Badge variant={ALERT_SEVERITY_BADGE_VARIANTS[alertEvent.severity]}>
                      {t(ALERT_SEVERITY_LABEL_KEYS[alertEvent.severity])}
                    </Badge>
                    <Badge variant={alertEvent.state === 'firing' ? 'destructive' : 'success'}>
                      {alertEvent.state === 'firing' ? t('alerts.stateFiring') : t('alerts.stateResolved')}
                    </Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">{alertEvent.message}</p>
                </div>

                <div className="flex shrink-0 items-center gap-2">
                  {alertEvent.deliveries.length === 0 ? (
                    <span className="text-xs text-muted-foreground">{t('alerts.deliveryInAppOnly')}</span>
                  ) : (
                    <>
                      {delivered > 0 ? (
                        <span
                          className="flex items-center gap-1 text-xs text-muted-foreground"
                          title={t('alerts.deliveryDelivered', { count: delivered })}
                        >
                          <IconCheck className="size-3.5 text-success" />
                          {delivered}
                        </span>
                      ) : null}
                      {failed > 0 ? (
                        <span
                          className="flex items-center gap-1 text-xs text-destructive"
                          title={t('alerts.deliveryFailed', { count: failed })}
                        >
                          <IconX className="size-3.5" />
                          {failed}
                        </span>
                      ) : null}
                    </>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
