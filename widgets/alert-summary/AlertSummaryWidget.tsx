// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { useAlerts } from '@modules/settings/hooks/useAlerts';

const SEVERITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500/15 text-red-400 border-red-500/20',
  warning: 'bg-yellow-500/15 text-yellow-400 border-yellow-500/20',
  info: 'bg-blue-500/15 text-blue-400 border-blue-500/20',
};

const SEVERITIES = ['critical', 'warning', 'info'] as const;

export default function AlertSummaryWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: alertsPage, isError } = useAlerts({ refetchInterval: 30_000 });

  const grouped: Record<string, number> = {};
  let disabledCount = 0;
  for (const alert of alertsPage?.items ?? []) {
    if (!alert.enabled) {
      disabledCount += 1;
      continue;
    }
    grouped[alert.severity] = (grouped[alert.severity] ?? 0) + 1;
  }

  if (isError) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('alertSummaryWidget.notConfigured')}
      </div>
    );
  }

  const total = Object.values(grouped).reduce((sum, count) => sum + count, 0);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pb-2 pt-3 text-sm font-semibold text-foreground">
        {t('alertSummaryWidget.title')}
      </h3>
      {total === 0 ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          {t('alertSummaryWidget.noAlerts')}
        </div>
      ) : (
        <div className="flex flex-1 flex-wrap content-start gap-2 px-4 pb-3">
          {SEVERITIES.map((severity) => {
            const count = grouped[severity] ?? 0;
            if (count === 0) return null;
            return (
              <div
                key={severity}
                className={`flex flex-col items-center rounded-lg border px-4 py-3 ${SEVERITY_COLORS[severity] ?? 'border-border bg-muted text-muted-foreground'}`}
              >
                <span className="text-2xl font-semibold">{count}</span>
                <span className="text-[10px]">{t(`alertSummaryWidget.severity.${severity}`)}</span>
              </div>
            );
          })}
        </div>
      )}
      {disabledCount > 0 ? (
        <p className="shrink-0 px-4 pb-3 text-xs text-muted-foreground">
          {t('alertSummaryWidget.disabledRules', { count: disabledCount })}
        </p>
      ) : null}
    </div>
  );
}
