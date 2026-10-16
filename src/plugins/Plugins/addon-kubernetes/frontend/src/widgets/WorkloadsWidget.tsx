// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// workloads — every deployment with its rollout progress, stalled ones first.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudOff, IconTopologyStar3 } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';
import {
  ROW_HEIGHT,
  TONE_SOLID,
  TONE_TEXT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { list } from '../format';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH, readyParts, useSummaryWidget } from './useSummaryWidget';

export function WorkloadsWidget() {
  const { t } = useTranslation('kubernetes');
  const { navigate } = useWidgetContext();
  const { data: summary, isLoading } = useSummaryWidget();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const rows = useMemo(() => {
    return (summary?.deployments ?? [])
      .map((row) => {
        const { ready, total } = readyParts(row);
        const share = total > 0 ? ready / total : 0;
        const tone: Tone = total === 0 ? 'neutral' : ready === 0 ? 'danger' : ready < total ? 'warning' : 'success';
        return { name: list(row.name), namespace: list(row.namespace), ready, total, share, tone };
      })
      .sort((a, b) => a.share - b.share || a.name.localeCompare(b.name));
  }, [summary]);

  const stalled = rows.filter((r) => r.tone !== 'success' && r.tone !== 'neutral').length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.workloads.title')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge tone={stalled > 0 ? 'warning' : 'success'}>
            {stalled > 0 ? t('widgets.workloads.stalled', { count: stalled }) : t('widgets.workloads.allReady')}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('tabs.deployments')} onClick={() => navigate(`${PLUGIN_PATH}/pods`)} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('widgets.common.loading')} />
      ) : !summary?.connected ? (
        <WidgetEmpty
          icon={<IconCloudOff className="size-5" aria-hidden="true" />}
          title={t('widgets.common.notConfigured')}
        />
      ) : rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconTopologyStar3 className="size-5" aria-hidden="true" />}
          title={t('widgets.workloads.none')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((row) => (
            <li key={`${row.namespace}/${row.name}`}>
              <WidgetRow
                title={row.name}
                subtitle={row.namespace}
                trailing={
                  <>
                    <span className="h-1.5 w-16 overflow-hidden rounded-full bg-glass-2" aria-hidden="true">
                      <span
                        className={cn('block h-full w-(--w) rounded-full', TONE_SOLID[row.tone])}
                        // The fill is the rollout itself; no utility class says "3 of 5 replicas".
                        style={cssVars({ w: `${Math.round(row.share * 100)}%` })}
                      />
                    </span>
                    <span
                      className={cn(
                        'w-10 text-right font-medium tabular-nums',
                        row.tone === 'success' ? 'text-foreground' : TONE_TEXT[row.tone],
                      )}
                    >
                      {row.ready}/{row.total}
                    </span>
                  </>
                }
                onClick={() => navigate(`${PLUGIN_PATH}/pods`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
