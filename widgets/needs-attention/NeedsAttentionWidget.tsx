// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// needs-attention — the things actually wanting a human.
//
// `fleet-health` says whether anything is wrong. This says what, ranked by
// consequence rather than recency: a failing health check beats an overdue
// snapshot beats a container that exited on its own. It shows as many rows
// as fit the tile and says how many it did not show.

import { useMemo, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  IconAlertTriangle,
  IconArchiveOff,
  IconCircleCheck,
  IconPlayerStop,
  IconRefreshAlert,
} from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { useAllContainerBackupPlans } from '@resources/hooks/useContainerBackups';
import { parseHealth } from '@modules/containers/components/container-utils';
import { useEnvironmentStore } from '@resources/stores/environment';
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
  type Tone,
} from '@modules/dashboard/components/widget-kit';

/** A snapshot that has not run in this long is overdue. */
const SNAPSHOT_OVERDUE_MS = 36 * 60 * 60 * 1000;

type Item = {
  id: string;
  /** Lower sorts first — the most consequential thing at the top. */
  rank: number;
  icon: typeof IconAlertTriangle;
  tone: Tone;
  title: string;
  detail: string;
  to: string;
};

export default function NeedsAttentionWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { data: containers = [], isError } = useContainers();
  const { data: plans } = useAllContainerBackupPlans(envId);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(id);
  }, []);

  const items = useMemo(() => {
    const out: Item[] = [];
    for (const container of containers) {
      const label = container.Names?.[0]?.replace(/^\//, '') ?? container.Id;
      const to = `/containers/${container.Id}`;
      if (parseHealth(container.Status) === 'unhealthy') {
        out.push({
          id: `unhealthy:${container.Id}`,
          rank: 0,
          icon: IconAlertTriangle,
          tone: 'danger',
          title: label,
          detail: t('needsAttentionWidget.reason.unhealthy'),
          to,
        });
      } else if (container.State === 'restarting') {
        out.push({
          id: `restarting:${container.Id}`,
          rank: 1,
          icon: IconRefreshAlert,
          tone: 'warning',
          title: label,
          detail: t('needsAttentionWidget.reason.restarting'),
          to,
        });
      } else if (container.State === 'dead') {
        out.push({
          id: `dead:${container.Id}`,
          rank: 1,
          icon: IconAlertTriangle,
          tone: 'critical',
          title: label,
          detail: t('needsAttentionWidget.reason.dead'),
          to,
        });
      } else if (container.State === 'exited') {
        out.push({
          id: `exited:${container.Id}`,
          rank: 3,
          icon: IconPlayerStop,
          tone: 'neutral',
          title: label,
          detail: t('needsAttentionWidget.reason.exited'),
          to,
        });
      }
    }

    for (const plan of plans) {
      if (!plan.enabled) continue;
      const last = plan.lastRunAt ? new Date(plan.lastRunAt).getTime() : 0;
      if (last && now - last < SNAPSHOT_OVERDUE_MS) continue;
      out.push({
        id: `snapshot:${plan.id}`,
        rank: 2,
        icon: IconArchiveOff,
        tone: 'warning',
        title: plan.containerName || plan.name,
        detail: last
          ? t('needsAttentionWidget.reason.snapshotOverdue')
          : t('needsAttentionWidget.reason.snapshotNever'),
        to: `/containers/${plan.containerId}?tab=backups`,
      });
    }

    out.sort((a, b) => a.rank - b.rank || a.title.localeCompare(b.title));
    return out;
  }, [containers, plans, t, now]);

  const shown = items.slice(0, capacity);
  const remaining = items.length - shown.length;
  const urgent = items.filter((i) => i.rank <= 1).length;

  return (
    <WidgetFrame
      title={t('needsAttentionWidget.title')}
      badge={
        items.length > 0 ? (
          <WidgetBadge tone={urgent > 0 ? 'danger' : 'warning'}>{items.length}</WidgetBadge>
        ) : undefined
      }
      footer={
        remaining > 0 ? (
          <>
            <span>{t('needsAttentionWidget.more', { count: remaining })}</span>
            <WidgetFooterLink
              label={t('needsAttentionWidget.viewAll')}
              onClick={() => navigate('/containers?state=unhealthy')}
            />
          </>
        ) : undefined
      }
    >
      {isError ? (
        <WidgetEmpty
          icon={<IconAlertTriangle className="size-5" aria-hidden="true" />}
          title={t('needsAttentionWidget.unavailable')}
        />
      ) : items.length === 0 ? (
        <WidgetEmpty
          tone="success"
          icon={<IconCircleCheck className="size-5" aria-hidden="true" />}
          title={t('needsAttentionWidget.empty')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((item) => (
            <li key={item.id}>
              <WidgetRow
                leading={
                  <IconTile tone={item.tone}>
                    <item.icon className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={item.title}
                subtitle={item.detail}
                onClick={() => navigate(item.to)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
