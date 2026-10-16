// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconBell, IconBellOff } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { useAlerts } from '@resources/hooks/useAlerts';
import {
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetTile,
  type Tone,
} from '@modules/dashboard/components/widget-kit';

const SEVERITIES: { key: string; tone: Tone }[] = [
  { key: 'critical', tone: 'danger' },
  { key: 'warning', tone: 'warning' },
  { key: 'info', tone: 'info' },
];

export default function AlertSummaryWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: alertsPage, isError } = useAlerts({ refetchInterval: 30_000 });

  const grouped: Record<string, number> = {};
  let disabledCount = 0;
  for (const rule of alertsPage?.items ?? []) {
    if (!rule.enabled) {
      disabledCount += 1;
      continue;
    }
    grouped[rule.severity] = (grouped[rule.severity] ?? 0) + 1;
  }
  const total = Object.values(grouped).reduce((sum, count) => sum + count, 0);
  const goToRules = () => navigate('/settings?tab=alerts');

  if (isError) {
    return (
      <WidgetFrame title={t('alertSummaryWidget.title')}>
        <WidgetEmpty
          icon={<IconBellOff className="size-5" aria-hidden="true" />}
          title={t('alertSummaryWidget.notConfigured')}
        />
      </WidgetFrame>
    );
  }

  return (
    <WidgetFrame
      title={t('alertSummaryWidget.title')}
      badge={total > 0 ? <WidgetBadge>{t('alertSummaryWidget.enabled', { count: total })}</WidgetBadge> : undefined}
      footer={
        <>
          <span>{disabledCount > 0 ? t('alertSummaryWidget.disabledRules', { count: disabledCount }) : null}</span>
          <WidgetFooterLink label={t('alertSummaryWidget.viewRules')} onClick={goToRules} />
        </>
      }
    >
      {total === 0 ? (
        <WidgetEmpty
          icon={<IconBell className="size-5" aria-hidden="true" />}
          title={t('alertSummaryWidget.noAlerts')}
        />
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-3 gap-2 px-4 pb-3">
          {SEVERITIES.map(({ key, tone }) => {
            const count = grouped[key] ?? 0;
            return (
              <WidgetTile
                key={key}
                label={t(`alertSummaryWidget.severity.${key}`)}
                value={count}
                tone={count > 0 ? tone : 'neutral'}
                share={total > 0 ? count / total : 0}
                onClick={goToRules}
              />
            );
          })}
        </div>
      )}
    </WidgetFrame>
  );
}
