// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// snapshot-status — last successful snapshot per target, and what is overdue.
//
// A backup you believe is running and is not is worse than no backup,
// because it removes the prompt to check. Overdue targets sort first, so
// the widget is useful precisely when it is bad news.

import { useMemo, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconArchive, IconArchiveOff } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { useAllContainerBackupPlans } from '@resources/hooks/useContainerBackups';
import { useEnvironmentStore } from '@resources/stores/environment';
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

const OVERDUE_MS = 36 * 60 * 60 * 1000;

export default function SnapshotStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data: plans } = useAllContainerBackupPlans(envId);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(id);
  }, []);

  const rows = useMemo(() => {
    return plans
      .filter((plan) => plan.enabled)
      .map((plan) => {
        const last = plan.lastRunAt ? new Date(plan.lastRunAt).getTime() : 0;
        return {
          id: plan.id,
          containerId: plan.containerId,
          name: plan.containerName || plan.name,
          age: last ? timeAgo(plan.lastRunAt) : t('snapshotStatusWidget.never'),
          overdue: !last || now - last > OVERDUE_MS,
          sortKey: last,
        };
      })
      .sort((a, b) => (a.overdue !== b.overdue ? (a.overdue ? -1 : 1) : a.sortKey - b.sortKey));
  }, [plans, t, now]);

  const overdueCount = rows.filter((row) => row.overdue).length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('snapshotStatusWidget.title')}
      badge={
        rows.length > 0 ? (
          overdueCount > 0 ? (
            <WidgetBadge tone="warning">{t('snapshotStatusWidget.overdueCount', { count: overdueCount })}</WidgetBadge>
          ) : (
            <WidgetBadge tone="success">{t('snapshotStatusWidget.allCurrent')}</WidgetBadge>
          )
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('snapshotStatusWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('snapshotStatusWidget.viewAll')} onClick={() => navigate('/backups')} />
        </>
      }
    >
      {rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconArchive className="size-5" aria-hidden="true" />}
          title={t('snapshotStatusWidget.noPlans')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((row) => (
            <li key={row.id}>
              <WidgetRow
                leading={
                  <IconTile tone={row.overdue ? 'warning' : 'success'}>
                    {row.overdue ? (
                      <IconArchiveOff className="size-4" aria-hidden="true" />
                    ) : (
                      <IconArchive className="size-4" aria-hidden="true" />
                    )}
                  </IconTile>
                }
                title={row.name}
                subtitle={row.overdue ? t('snapshotStatusWidget.overdue') : t('snapshotStatusWidget.current')}
                trailing={<span className={row.overdue ? 'text-warning tabular-nums' : 'tabular-nums'}>{row.age}</span>}
                onClick={() => navigate(`/containers/${row.containerId}?tab=backups`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
