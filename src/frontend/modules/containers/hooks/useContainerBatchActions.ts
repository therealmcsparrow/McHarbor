// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlay, IconPlayerStop, IconRotate, IconArrowUp, IconTrash } from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import { canRunContainerUpdateOperation, isProtectedContainer } from '@core/utils/protection';
import type { BatchAction } from '@resources/components/DataGrid';

type UseContainerBatchActionsProps = {
  action: { mutate: (vars: { id: string; action: string }) => void };
  onUpdateSelected?: (rows: ContainerInfo[]) => void;
  onReinstallSelected?: (rows: ContainerInfo[]) => void;
  onRemoveSelected?: (rows: ContainerInfo[]) => void;
};

export function useContainerBatchActions({
  action,
  onUpdateSelected,
  onReinstallSelected,
  onRemoveSelected,
}: UseContainerBatchActionsProps) {
  const { t: tc } = useTranslation('common');

  return useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.start'),
        icon: IconPlayerPlay,
        iconClassName: 'text-emerald-500',
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
        iconClassName: 'text-amber-500',
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
        iconClassName: 'text-blue-400',
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
        iconClassName: 'text-emerald-400',
        variant: 'default',
        onClick: (rows) => onUpdateSelected?.((rows as ContainerInfo[]).filter(canRunContainerUpdateOperation)),
      },
      {
        label: tc('batch.reinstallSelected'),
        icon: IconRotate,
        iconClassName: 'text-blue-400',
        variant: 'default',
        onClick: (rows) => onReinstallSelected?.((rows as ContainerInfo[]).filter(canRunContainerUpdateOperation)),
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
    [tc, action, onUpdateSelected, onReinstallSelected, onRemoveSelected]
  );
}
