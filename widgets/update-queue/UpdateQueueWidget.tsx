// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// update-queue — what has an image update ready, and the button to do it.
//
// The check is not run automatically here: probing every registry is a
// network cost the user should choose to pay, so this shows what the last
// check found and offers to check again.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconArrowUp, IconCircleCheck, IconRefresh } from '@tabler/icons-react';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { useCheckContainerUpdates, useContainerUpdateResults } from '@modules/containers/hooks/useContainerUpdates';
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

export default function UpdateQueueWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers = [] } = useContainers();
  const updateResults = useContainerUpdateResults();
  const checkUpdates = useCheckContainerUpdates();
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const pending = useMemo(() => {
    if (!updateResults) return [];
    const ready = new Set(
      [...updateResults.values()].filter((result) => result.updateAvailable).map((result) => result.containerId),
    );
    return containers
      .filter((container) => ready.has(container.Id))
      .map((container) => ({
        id: container.Id,
        name: container.Names?.[0]?.replace(/^\//, '') ?? container.Id,
        image: container.Image,
      }));
  }, [containers, updateResults]);

  const neverChecked = !updateResults;
  const shown = pending.slice(0, capacity);
  const remaining = pending.length - shown.length;

  const refresh = (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={t('updateQueueWidget.checkAgain')}
      title={t('updateQueueWidget.checkAgain')}
      disabled={checkUpdates.isPending}
      onClick={() => checkUpdates.mutate(undefined)}
    >
      {checkUpdates.isPending ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
    </Button>
  );

  return (
    <WidgetFrame
      title={t('updateQueueWidget.title')}
      badge={pending.length > 0 ? <WidgetBadge tone="info">{pending.length}</WidgetBadge> : undefined}
      action={refresh}
      footer={
        pending.length > 0 ? (
          <>
            <span>{remaining > 0 ? t('updateQueueWidget.more', { count: remaining }) : null}</span>
            {/* Lands on the containers page with the update-ready facet applied,
                so the user reviews the set before committing. */}
            <WidgetFooterLink
              label={t('updateQueueWidget.reviewAll', { count: pending.length })}
              onClick={() => navigate('/containers?state=update')}
            />
          </>
        ) : undefined
      }
    >
      {neverChecked ? (
        <WidgetEmpty
          icon={<IconRefresh className="size-5" aria-hidden="true" />}
          title={t('updateQueueWidget.notChecked')}
          action={
            <Button
              variant="outline"
              size="sm"
              disabled={checkUpdates.isPending}
              onClick={() => checkUpdates.mutate(undefined)}
            >
              {checkUpdates.isPending ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
              {t('updateQueueWidget.checkNow')}
            </Button>
          }
        />
      ) : pending.length === 0 ? (
        <WidgetEmpty
          tone="success"
          icon={<IconCircleCheck className="size-5" aria-hidden="true" />}
          title={t('updateQueueWidget.upToDate')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((item) => (
            <li key={item.id}>
              <WidgetRow
                leading={
                  <IconTile tone="info">
                    <IconArrowUp className="size-4" aria-hidden="true" />
                  </IconTile>
                }
                title={item.name}
                subtitle={<span className="font-mono">{item.image}</span>}
                onClick={() => navigate(`/containers/${item.id}?tab=updates`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
