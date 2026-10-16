// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// TableToolbar — the band that used to be empty.
//
// On the audit page, the room below "Save filter" and to the right of the
// predefined-filter selectors was dead space. This is what fills it: a slot for
// whatever the page itself needs there, then the controls that act on the *view*
// rather than on the data — what is shown (columns), how tightly (density), how
// fresh (refresh), and how to take it away (export).
//
// Page actions come first because they are what the page is *for*; the view
// controls are chrome. A divider separates the two groups so the band does not
// read as one undifferentiated row of icons.

import type { RowData, Table } from '@resources/components/table-features';
import { TableEditSheet } from './TableEditSheet';
import { TableDensityMenu } from './TableDensityMenu';
import { TableRefreshControl } from './TableRefreshControl';
import { TableExportButton } from './TableExportButton';
import type { TableView } from './useTableView';
import type { TablePageToolbar } from './types';

type TableToolbarProps<T extends RowData> = {
  config: TablePageToolbar;
  table: Table<T>;
  view: TableView;
  serverPaged: boolean;
};

export function TableToolbar<T extends RowData>({ config, table, view, serverPaged }: TableToolbarProps<T>) {
  const hasViewControls = Boolean(config.columns || config.density || config.refresh || config.export);
  const hasPageActions = Boolean(config.actions);

  if (!hasViewControls && !hasPageActions) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {config.actions}
      {hasPageActions && hasViewControls && <div aria-hidden="true" className="mx-0.5 h-5 w-px bg-border" />}
      {config.refresh && (
        <TableRefreshControl
          onRefresh={config.refresh.onRefresh}
          isFetching={config.refresh.isFetching}
          intervalMs={view.autoRefreshMs}
          onIntervalChange={view.setAutoRefreshMs}
        />
      )}
      {config.columns && <TableEditSheet table={table} view={view} />}
      {config.density && (
        <TableDensityMenu value={view.density} onChange={view.setDensity} disabled={!view.available} />
      )}
      {config.export && (
        <TableExportButton
          table={table}
          filename={config.export.filename}
          server={config.export.server}
          serverPaged={serverPaged}
        />
      )}
    </div>
  );
}
