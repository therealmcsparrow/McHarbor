// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconCircleCheck, IconGitCompare, IconLock } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useDriftReport, type DriftReportRow } from '@modules/reconciler/hooks/useDriftReport';
import { timeAgo } from '@resources/utils/format';
import {
  IconTile,
  ROW_HEIGHT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetRow,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';

export type DriftedWatch = {
  id: string;
  name: string;
  classes: string[];
  count: number;
  firstSeenAt: string;
};

export function groupByWatch(rows: DriftReportRow[]): DriftedWatch[] {
  const byId = new Map<string, DriftedWatch>();
  for (const row of rows) {
    const watch = byId.get(row.desiredStateId) ?? {
      id: row.desiredStateId,
      name: row.name,
      classes: [],
      count: 0,
      firstSeenAt: row.firstSeenAt,
    };
    if (!watch.classes.includes(row.class)) watch.classes.push(row.class);
    watch.count += 1;
    if (row.firstSeenAt && (!watch.firstSeenAt || row.firstSeenAt < watch.firstSeenAt))
      watch.firstSeenAt = row.firstSeenAt;
    byId.set(row.desiredStateId, watch);
  }
  return [...byId.values()].sort((a, b) => a.firstSeenAt.localeCompare(b.firstSeenAt));
}

export default function DriftStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { t: tr } = useTranslation('reconciler');
  const navigate = useNavigate();
  const allowed = usePermission(PERM.RECONCILER_VIEW);
  const report = useDriftReport('', allowed);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const watches = useMemo(() => (allowed ? groupByWatch(report.data ?? []) : []), [allowed, report.data]);
  const shown = watches.slice(0, capacity);
  const remaining = watches.length - shown.length;

  return (
    <WidgetFrame
      title={t('driftStatusWidget.title')}
      badge={watches.length > 0 ? <WidgetBadge tone="warning">{watches.length}</WidgetBadge> : undefined}
      footer={
        allowed && watches.length > 0 ? (
          <>
            {remaining > 0 ? <span>{t('driftStatusWidget.more', { count: remaining })}</span> : <span />}
            <WidgetFooterLink
              label={t('driftStatusWidget.viewReport')}
              onClick={() => navigate('/reconciler/report')}
            />
          </>
        ) : undefined
      }
    >
      {!allowed ? (
        <WidgetEmpty
          icon={<IconLock className="size-5" aria-hidden="true" />}
          title={t('driftStatusWidget.noAccess')}
        />
      ) : report.isError ? (
        <WidgetEmpty
          icon={<IconGitCompare className="size-5" aria-hidden="true" />}
          title={t('driftStatusWidget.unavailable')}
        />
      ) : watches.length === 0 ? (
        <WidgetEmpty
          tone="success"
          icon={<IconCircleCheck className="size-5" aria-hidden="true" />}
          title={t('driftStatusWidget.empty')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((watch) => (
            <li key={watch.id}>
              <WidgetRow
                leading={
                  <IconTile tone="warning">
                    <IconGitCompare className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={watch.name}
                subtitle={t('driftStatusWidget.detail', {
                  classes: watch.classes.map((c) => tr(`class.${c}`)).join(', '),
                  since: watch.firstSeenAt ? timeAgo(watch.firstSeenAt) : '—',
                })}
                onClick={() => navigate('/reconciler/report')}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
