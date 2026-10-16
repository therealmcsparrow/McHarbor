// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLayoutColumns, IconPlus } from '@tabler/icons-react';
import type { RowData, Table } from '@resources/components/table-features';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { TableEditAvailableList } from './TableEditAvailableList';
import { TableEditShownList } from './TableEditShownList';
import { useTableEditDraft } from './useTableEditDraft';
import type { TableView } from './useTableView';
import type { TableColumnMeta } from './types';

type TableEditSheetProps<T extends RowData> = { table: Table<T>; view: TableView };

export function TableEditSheet<T extends RowData>({ table, view }: TableEditSheetProps<T>) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const edit = useTableEditDraft(view, isOpen);

  const { labels, groups } = useMemo(() => {
    const labelMap: Record<string, string> = {};
    const groupMap: Record<string, string | undefined> = {};
    for (const column of table.getAllLeafColumns()) {
      const meta = column.columnDef.meta as TableColumnMeta | undefined;
      const header = column.columnDef.header;
      labelMap[column.id] = meta?.label ?? (typeof header === 'string' ? header : column.id);
      groupMap[column.id] = meta?.group;
    }
    return { labels: labelMap, groups: groupMap };
  }, [table]);

  const save = () => {
    view.saveLayout({ resolved: edit.draft });
    setOpen(false);
  };

  const trigger = (
    <Button
      variant="outline"
      size="sm"
      disabled={!view.available}
      onClick={() => {
        setOpen(true);
      }}
    >
      <IconLayoutColumns className="size-4" />
      {t('tableViews.editTable')}
    </Button>
  );

  const hasHidden = edit.hidden.length > 0;

  const addColumnButton = (
    <Button variant="outline" size="sm" disabled={!hasHidden}>
      <IconPlus className="size-4" />
      {t('tableViews.addColumnButton')}
    </Button>
  );

  return (
    <>
      {view.available ? (
        trigger
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>
            <TooltipFocusTarget>{trigger}</TooltipFocusTarget>
          </TooltipTrigger>
          <TooltipContent>{t('tableViews.unavailable')}</TooltipContent>
        </Tooltip>
      )}
      <Dialog open={isOpen} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t('tableViews.editTable')}</DialogTitle>
            <DialogDescription>{t('tableViews.editTableDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2">
              <h2 className="text-sm font-medium text-foreground">{t('tableViews.shown')}</h2>
              {hasHidden ? (
                <Popover>
                  <PopoverTrigger asChild>{addColumnButton}</PopoverTrigger>
                  <PopoverContent align="end" className="w-80">
                    <h3 className="mb-2 text-xs font-medium text-muted-foreground">{t('tableViews.available')}</h3>
                    <TableEditAvailableList ids={edit.hidden} labels={labels} groups={groups} onShow={edit.show} />
                  </PopoverContent>
                </Popover>
              ) : (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <TooltipFocusTarget>{addColumnButton}</TooltipFocusTarget>
                  </TooltipTrigger>
                  <TooltipContent>{t('tableViews.noAvailable')}</TooltipContent>
                </Tooltip>
              )}
            </div>
            <TableEditShownList
              ids={edit.shown}
              labels={labels}
              sizes={edit.draft.sizes}
              headerAlign={edit.draft.headerAlign}
              cellAlign={edit.draft.cellAlign}
              sortable={edit.draft.sortable}
              sortCapable={edit.sortCapable}
              locked={edit.locked}
              onMove={edit.move}
              onMoveTo={edit.moveTo}
              onHide={edit.hide}
              onResize={edit.resize}
              onAlign={edit.align}
              onSortable={edit.setSortable}
            />
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" className="mr-auto" onClick={() => setConfirmReset(true)}>
              {t('tableViews.reset')}
            </Button>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('actions.cancel')}
            </Button>
            <Button onClick={save}>{t('actions.save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={t('tableViews.reset')}
        description={t('tableViews.resetConfirm')}
        confirmLabel={t('tableViews.reset')}
        variant="caution"
        onConfirm={() => {
          view.resetLayout();
          edit.reset();
          setConfirmReset(false);
          setOpen(false);
        }}
      />
    </>
  );
}
