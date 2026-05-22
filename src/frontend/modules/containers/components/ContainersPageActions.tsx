// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconArrowUp, IconLayoutGrid, IconLayoutList, IconPlus, IconRefresh, IconRotate } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';

type ContainersPageActionsProps = {
  viewMode: 'table' | 'card';
  checkingUpdates: boolean;
  batchRunning: boolean;
  updatesAvailable: number;
  totalContainers: number;
  onCheckUpdates: () => void;
  onUpdateAll: () => void;
  onReinstallAll: () => void;
  onCreate: () => void;
  onViewModeChange: (viewMode: 'table' | 'card') => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function ContainersPageActions({
  viewMode,
  checkingUpdates,
  batchRunning,
  updatesAvailable,
  totalContainers,
  onCheckUpdates,
  onUpdateAll,
  onReinstallAll,
  onCreate,
  onViewModeChange,
  t,
}: ContainersPageActionsProps) {
  return (
    <>
      <Button variant="outline" onClick={onCheckUpdates} disabled={checkingUpdates || batchRunning}>
        {checkingUpdates ? <Spinner size="sm" /> : <IconRefresh className="h-4 w-4" />}
        {t('updates.searchForUpdates')}
      </Button>
      {updatesAvailable > 0 && (
        <Button variant="outline" onClick={onUpdateAll} disabled={batchRunning}>
          {batchRunning ? <Spinner size="sm" /> : <IconArrowUp className="h-4 w-4" />}
          {t('updates.updateAll', { count: updatesAvailable })}
        </Button>
      )}
      {totalContainers > 0 && (
        <Button variant="outline" onClick={onReinstallAll} disabled={batchRunning}>
          {batchRunning ? <Spinner size="sm" /> : <IconRotate className="h-4 w-4" />}
          {t('updates.reinstallAll', { count: totalContainers })}
        </Button>
      )}
      <Button onClick={onCreate}>
        <IconPlus className="h-4 w-4" /> {t('createContainer')}
      </Button>
      <div className="h-6 w-px bg-border" />
      <div className="flex items-center rounded-lg border border-border">
        <Button
          variant={viewMode === 'table' ? 'default' : 'ghost'}
          size="icon-sm"
          onClick={() => onViewModeChange('table')}
          aria-label={t('tableView')}
        >
          <IconLayoutList className="h-4 w-4" />
        </Button>
        <Button
          variant={viewMode === 'card' ? 'default' : 'ghost'}
          size="icon-sm"
          onClick={() => onViewModeChange('card')}
          aria-label={t('cardView')}
        >
          <IconLayoutGrid className="h-4 w-4" />
        </Button>
      </div>
    </>
  );
}
