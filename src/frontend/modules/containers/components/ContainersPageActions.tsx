// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconArrowUp,
  IconBlocks,
  IconFilterOff,
  IconLayoutGrid,
  IconLayoutList,
  IconPlus,
  IconRefresh,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';

// Shared prop set — every field is needed by BOTH the topbar
// (header scope) and the table-toolbar (toolbar scope) renders
// EXCEPT for `onCreate` which only the toolbar needs. The view-mode
// toggle lives in the topbar (header scope) only — toolbar actions
// are bulk operations + create.
type CommonProps = {
  /** Required for the header scope (view-mode toggle). Optional for the toolbar. */
  viewMode?: 'table' | 'card';
  checkingUpdates: boolean;
  batchRunning: boolean;
  uploadActive: boolean;
  updatesAvailable: number;
  totalContainers: number;
  onCheckUpdates: () => void;
  onUpdateAll: () => void;
  onRecreateAll: () => void;
  onPruneUnused: () => void;
  /** Required for the header scope. Optional for the toolbar. */
  onViewModeChange?: (viewMode: 'table' | 'card') => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

type ToolbarProps = CommonProps & {
  scope: 'toolbar';
  /** Create-container handler. Only present in the toolbar scope. */
  onCreate: () => void;
};

type HeaderProps = CommonProps & {
  scope: 'header';
};

// Discriminated union — the compiler will reject a header-scope call
// site that forgets `onCreate`, and a toolbar-scope call site that
// forgets `onCreate`. No optional-flag footguns.
export type ContainersPageActionsProps = ToolbarProps | HeaderProps;

// Toolbar layout (right of the search field, above the table):
//   [Create container (primary)] | [Check for updates] [Recreate All] [Prune unused]
// The view-mode toggle lives in the topbar so the toolbar stays
// focused on bulk operations + create.
function renderToolbar(props: ToolbarProps) {
  const {
    checkingUpdates,
    batchRunning,
    uploadActive,
    updatesAvailable,
    totalContainers,
    onCheckUpdates,
    onUpdateAll,
    onRecreateAll,
    onPruneUnused,
    onCreate,
    t,
  } = props;
  return (
    <>
      <Button onClick={onCreate}>
        <IconPlus className="h-4 w-4" /> {t('createContainer')}
      </Button>
      <div className="h-6 w-px bg-border" />
      <Button
        variant="outline"
        size="sm"
        onClick={onCheckUpdates}
        disabled={checkingUpdates || batchRunning || uploadActive}
      >
        {checkingUpdates ? <Spinner size="sm" /> : <IconRefresh className="h-4 w-4" />}
        {t('updates.searchForUpdates')}
      </Button>
      {totalContainers > 0 && (
        <Button variant="outline" size="sm" onClick={onRecreateAll} disabled={batchRunning || uploadActive}>
          {batchRunning ? <Spinner size="sm" /> : <IconBlocks className="h-4 w-4" />}
          {t('updates.recreateAll', { count: totalContainers })}
        </Button>
      )}
      <Button variant="outline" size="sm" onClick={onPruneUnused} disabled={batchRunning || uploadActive}>
        <IconFilterOff className="h-4 w-4" />
        {t('pruneUnused')}
      </Button>
      {updatesAvailable > 0 && (
        <Button variant="outline" size="sm" onClick={onUpdateAll} disabled={batchRunning || uploadActive}>
          {batchRunning ? <Spinner size="sm" /> : <IconArrowUp className="h-4 w-4" />}
          {t('updates.updateAll', { count: updatesAvailable })}
        </Button>
      )}
    </>
  );
}

// Header layout: the view-mode toggle, and nothing else.
//
// It used to carry Update All as well. TablePage now hoists the toolbar scope
// into this same header band, and that scope already renders Update All — so
// keeping it here would put the button on the line twice.
function renderHeader(props: HeaderProps) {
  const { viewMode, onViewModeChange, t } = props;
  return (
    <ViewModeToggle
      viewMode={viewMode as 'table' | 'card'}
      onViewModeChange={onViewModeChange as (v: 'table' | 'card') => void}
      t={t}
    />
  );
}

function ViewModeToggle({
  viewMode,
  onViewModeChange,
  t,
}: {
  viewMode: 'table' | 'card';
  onViewModeChange: (v: 'table' | 'card') => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  // Lightweight inline toggle: two buttons, table | card.
  return (
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
  );
}

export function ContainersPageActions(props: ContainersPageActionsProps) {
  if (props.scope === 'toolbar') {
    return renderToolbar(props);
  }
  return renderHeader(props);
}
