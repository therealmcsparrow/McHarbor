// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconArrowUp, IconFilterOff, IconLayoutGrid, IconLayoutList, IconPlus, IconRefresh, IconRotate } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';

type ContainersPageActionsProps = {
  scope?: 'header' | 'toolbar';
  viewMode: 'table' | 'card';
  checkingUpdates: boolean;
  batchRunning: boolean;
  uploadActive: boolean;
  updatesAvailable: number;
  totalContainers: number;
  onCheckUpdates: () => void;
  onUpdateAll: () => void;
  onReinstallAll: () => void;
  onPruneUnused: () => void;
  onCreate: () => void;
  onViewModeChange: (viewMode: 'table' | 'card') => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function ContainersPageActions({
  scope = 'header',
  viewMode,
  checkingUpdates,
  batchRunning,
  uploadActive,
  updatesAvailable,
  totalContainers,
  onCheckUpdates,
  onUpdateAll,
  onReinstallAll,
  onPruneUnused,
  onCreate,
  onViewModeChange,
  t,
}: ContainersPageActionsProps) {
  if (scope === 'toolbar') {
    return (
      <>
        <Button variant="outline" size="sm" onClick={onCheckUpdates} disabled={checkingUpdates || batchRunning || uploadActive}>
          {checkingUpdates ? <Spinner size="sm" /> : <IconRefresh className="h-4 w-4" />}
          {t('updates.searchForUpdates')}
        </Button>
        {totalContainers > 0 && (
          <Button variant="outline" size="sm" onClick={onReinstallAll} disabled={batchRunning || uploadActive}>
            {batchRunning ? <Spinner size="sm" /> : <IconRotate className="h-4 w-4" />}
            {t('updates.reinstallAll', { count: totalContainers })}
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={onPruneUnused} disabled={batchRunning || uploadActive}>
          <IconFilterOff className="h-4 w-4" />
          {t('pruneUnused')}
        </Button>
      </>
    );
  }

  return (
    <>
      {updatesAvailable > 0 && (
        <Button variant="outline" onClick={onUpdateAll} disabled={batchRunning || uploadActive}>
          {batchRunning ? <Spinner size="sm" /> : <IconArrowUp className="h-4 w-4" />}
          {t('updates.updateAll', { count: updatesAvailable })}
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
