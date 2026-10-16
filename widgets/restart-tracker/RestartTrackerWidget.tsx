// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconCircleCheck, IconRefreshAlert } from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  IconTile,
  ROW_HEIGHT,
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
} from '@modules/dashboard/components/widget-kit';

export default function RestartTrackerWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useContainers(true);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const sorted = useMemo(() => {
    return (containers ?? [])
      .map((c) => {
        const match = (c.Status ?? '').match(/(\d+)\s*(times|time)/i);
        return { ...c, restartCount: match?.[1] ? parseInt(match[1], 10) : 0 };
      })
      .filter((c) => c.restartCount > 0 || c.State === 'restarting')
      .sort((a, b) => b.restartCount - a.restartCount);
  }, [containers]);

  const totalRestarts = sorted.reduce((sum, c) => sum + c.restartCount, 0);
  const shown = sorted.slice(0, capacity);
  const remaining = sorted.length - shown.length;

  return (
    <WidgetFrame
      title={t('restartTrackerWidget.title')}
      badge={
        sorted.length > 0 ? (
          <WidgetBadge tone="warning">{t('restartTrackerWidget.restartCount', { count: totalRestarts })}</WidgetBadge>
        ) : undefined
      }
      footer={remaining > 0 ? <span>{t('restartTrackerWidget.more', { count: remaining })}</span> : undefined}
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : sorted.length === 0 ? (
        <WidgetEmpty
          tone="success"
          icon={<IconCircleCheck className="size-5" aria-hidden="true" />}
          title={t('restartTrackerWidget.noRestarts')}
          hint={t('restartTrackerWidget.allStable')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((c) => (
            <li key={c.Id}>
              <WidgetRow
                leading={
                  <IconTile tone={c.State === 'restarting' ? 'danger' : 'warning'}>
                    <IconRefreshAlert className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={(c.Names?.[0] ?? '').replace(/^\//, '')}
                subtitle={c.Status}
                trailing={
                  <>
                    <span className="font-semibold text-warning tabular-nums">
                      {t('restartTrackerWidget.restartCount', { count: c.restartCount })}
                    </span>
                    <StatusBadge
                      status={c.State ?? 'unknown'}
                      map={CONTAINER_STATUS}
                      className="px-1.5 py-0.5 text-[10px]"
                    />
                  </>
                }
                onClick={() => navigate(`/containers/${c.Id}`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
