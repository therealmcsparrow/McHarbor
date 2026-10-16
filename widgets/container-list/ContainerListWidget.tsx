// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconBoxOff } from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import {
  ROW_HEIGHT,
  ToneDot,
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

const STATE_TONE: Record<string, Tone> = {
  running: 'success',
  restarting: 'warning',
  paused: 'warning',
  created: 'info',
  exited: 'danger',
  dead: 'critical',
};

/** Running first, then whatever needs a look, then the rest by name. */
const STATE_RANK: Record<string, number> = { restarting: 0, dead: 0, paused: 1, running: 2, created: 3, exited: 4 };

export default function ContainerListWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers = [], isLoading } = useContainers(true);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const sorted = [...containers].sort((a, b) => {
    const ra = STATE_RANK[a.State ?? ''] ?? 9;
    const rb = STATE_RANK[b.State ?? ''] ?? 9;
    return ra - rb || (a.Names?.[0] ?? '').localeCompare(b.Names?.[0] ?? '');
  });
  const running = containers.filter((c) => c.State === 'running').length;
  const shown = sorted.slice(0, capacity);
  const remaining = sorted.length - shown.length;

  return (
    <WidgetFrame
      title={t('containerListWidget.title')}
      badge={
        containers.length > 0 ? (
          <WidgetBadge tone={running === containers.length ? 'success' : 'neutral'}>
            {t('containerListWidget.running', { running, total: containers.length })}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('containerListWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('containerListWidget.viewAllArrow')} onClick={() => navigate('/containers')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('containerListWidget.loadingContainers')} />
      ) : containers.length === 0 ? (
        <WidgetEmpty
          icon={<IconBoxOff className="size-5" aria-hidden="true" />}
          title={t('containerListWidget.noContainersFound')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((c) => (
            <li key={c.Id}>
              <WidgetRow
                leading={<ToneDot tone={STATE_TONE[c.State ?? ''] ?? 'neutral'} />}
                title={(c.Names?.[0] ?? '').replace(/^\//, '')}
                subtitle={c.Image?.split('@')[0]}
                trailing={
                  <StatusBadge
                    status={c.State ?? 'unknown'}
                    map={CONTAINER_STATUS}
                    className="px-1.5 py-0.5 text-[10px]"
                  />
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
