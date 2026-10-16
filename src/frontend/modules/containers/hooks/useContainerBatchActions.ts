// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlay, IconPlayerStop, IconRotate, IconBlocks, IconArrowUp, IconTrash } from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { canRunContainerUpdateOperation, isProtectedContainer } from '@core/utils/protection';
import type { BatchAction } from '@resources/layout/table-page/types';

type UseContainerBatchActionsProps = {
  action: { mutate: (vars: { id: string; action: string }) => void };
  onUpdateSelected?: (rows: ContainerInfo[]) => void;
  onRecreateSelected?: (rows: ContainerInfo[]) => void;
  onRemoveSelected?: (rows: ContainerInfo[]) => void;
};

export function useContainerBatchActions({
  action,
  onUpdateSelected,
  onRecreateSelected,
  onRemoveSelected,
}: UseContainerBatchActionsProps) {
  const { t: tc } = useTranslation('common');

  return useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.start'),
        icon: IconPlayerPlay,
        iconClassName: 'text-success',
        variant: 'default',
        onClick: (rows) => {
          for (const row of rows as ContainerInfo[]) {
            if (isProtectedContainer(row)) continue;
            action.mutate({ id: row.Id, action: 'start' });
          }
        },
      },
      {
        label: tc('batch.stop'),
        icon: IconPlayerStop,
        iconClassName: 'text-warning',
        variant: 'default',
        onClick: (rows) => {
          for (const row of rows as ContainerInfo[]) {
            if (isProtectedContainer(row)) continue;
            action.mutate({ id: row.Id, action: 'stop' });
          }
        },
      },
      {
        label: tc('batch.restart'),
        icon: IconRotate,
        iconClassName: 'text-info',
        variant: 'default',
        onClick: (rows) => {
          for (const row of rows as ContainerInfo[]) {
            if (isProtectedContainer(row)) continue;
            action.mutate({ id: row.Id, action: 'restart' });
          }
        },
      },
      {
        label: tc('batch.updateSelected'),
        icon: IconArrowUp,
        iconClassName: 'text-success',
        variant: 'default',
        onClick: (rows) => onUpdateSelected?.((rows as ContainerInfo[]).filter(canRunContainerUpdateOperation)),
      },
      {
        label: tc('batch.recreateSelected'),
        icon: IconBlocks,
        iconClassName: 'text-info',
        variant: 'default',
        onClick: (rows) => onRecreateSelected?.((rows as ContainerInfo[]).filter(canRunContainerUpdateOperation)),
      },
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        iconClassName: 'text-destructive',
        variant: 'destructive',
        onClick: (rows) => {
          const eligible = (rows as ContainerInfo[]).filter((c) => !isProtectedContainer(c));
          onRemoveSelected?.(eligible);
        },
      },
    ],
    [tc, action, onUpdateSelected, onRecreateSelected, onRemoveSelected],
  );
}
