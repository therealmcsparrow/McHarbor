// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// events — the cluster's latest events, warnings first.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCloudOff, IconInfoCircle } from '@tabler/icons-react';
import {
  IconTile,
  ROW_HEIGHT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';
import { list } from '../format';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH, useSummaryWidget } from './useSummaryWidget';

export function EventsWidget() {
  const { t } = useTranslation('kubernetes');
  const { navigate } = useWidgetContext();
  const { data: summary, isLoading } = useSummaryWidget();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const rows = useMemo(() => {
    return (
      (summary?.events ?? [])
        .map((row, index) => ({
          id: `${list(row.namespace)}/${list(row.object)}/${list(row.reason)}/${index}`,
          warning: list(row.type) === 'Warning',
          reason: list(row.reason),
          object: list(row.object),
          namespace: list(row.namespace),
          message: list(row.message),
          age: list(row.age),
        }))
        // Warnings first; the server already orders by recency within a type.
        .sort((a, b) => Number(b.warning) - Number(a.warning))
    );
  }, [summary]);

  const warnings = rows.filter((r) => r.warning).length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.events.title')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge tone={warnings > 0 ? 'warning' : 'success'}>
            {warnings > 0 ? t('widgets.events.warnings', { count: warnings }) : t('widgets.events.noWarnings')}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('tabs.events')} onClick={() => navigate(`${PLUGIN_PATH}/nodes`)} />
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
          tone="success"
          icon={<IconInfoCircle className="size-5" aria-hidden="true" />}
          title={t('widgets.events.none')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((row) => (
            <li key={row.id}>
              <WidgetRow
                leading={
                  <IconTile tone={row.warning ? 'warning' : 'info'}>
                    {row.warning ? (
                      <IconAlertTriangle className="size-4" aria-hidden="true" />
                    ) : (
                      <IconInfoCircle className="size-4" aria-hidden="true" />
                    )}
                  </IconTile>
                }
                title={
                  <>
                    <span className="font-medium">{row.reason}</span>
                    <span className="text-muted-foreground"> · {row.object}</span>
                  </>
                }
                subtitle={row.message}
                trailing={<span className="tabular-nums">{row.age}</span>}
                onClick={() => navigate(`${PLUGIN_PATH}/nodes`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
