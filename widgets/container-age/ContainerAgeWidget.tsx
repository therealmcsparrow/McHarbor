// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconBoxOff, IconClock } from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { timeAgo } from '@resources/utils/format';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
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

const DAY_MS = 24 * 60 * 60 * 1000;

export default function ContainerAgeWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers = [], isLoading } = useContainers(true);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(id);
  }, []);

  const sorted = useMemo(() => [...containers].sort((a, b) => a.Created - b.Created), [containers]);
  const shown = sorted.slice(0, capacity);
  const remaining = sorted.length - shown.length;

  return (
    <WidgetFrame
      title={t('containerAgeWidget.title')}
      badge={sorted.length > 0 ? <WidgetBadge>{t('containerAgeWidget.oldestFirst')}</WidgetBadge> : undefined}
      footer={
        <>
          <span>{remaining > 0 ? t('containerAgeWidget.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('containerAgeWidget.viewAll')} onClick={() => navigate('/containers')} />
        </>
      }
    >
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : sorted.length === 0 ? (
        <WidgetEmpty
          icon={<IconBoxOff className="size-5" aria-hidden="true" />}
          title={t('containerAgeWidget.noContainers')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((c) => {
            const ageMs = now - c.Created * 1000;
            const days = Math.floor(ageMs / DAY_MS);
            return (
              <li key={c.Id}>
                <WidgetRow
                  leading={
                    <IconTile tone={days > 90 ? 'warning' : 'neutral'}>
                      <IconClock className="size-4" aria-hidden="true" />
                    </IconTile>
                  }
                  title={(c.Names?.[0] ?? '').replace(/^\//, '')}
                  subtitle={t('containerAgeWidget.createdAgo', {
                    when: timeAgo(new Date(c.Created * 1000).toISOString()),
                  })}
                  trailing={
                    <>
                      <span className="text-foreground tabular-nums">
                        {t('containerAgeWidget.days', { count: days })}
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
            );
          })}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
