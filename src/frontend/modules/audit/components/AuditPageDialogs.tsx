// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { AuditEntryDetails } from '@resources/components/AuditEntryDetails';
import { AuditExportDialog } from './AuditExportDialog';
import { AuditActionsDialog } from './AuditActionsDialog';
import type { useAuditPage } from '../hooks/useAuditPage';

type AuditPageDialogsProps = Pick<
  ReturnType<typeof useAuditPage>,
  | 'filters'
  | 'selected'
  | 'setSelected'
  | 'exportOpen'
  | 'setExportOpen'
  | 'actionsOpen'
  | 'setActionsOpen'
  | 'actionModuleFilter'
  | 'setActionModuleFilter'
  | 'envName'
>;

// The three overlays AuditPage owns: the entry detail sheet, the action picker
// and the export dialog. Split out so the page itself stays a layout.
export function AuditPageDialogs({
  filters,
  selected,
  setSelected,
  exportOpen,
  setExportOpen,
  actionsOpen,
  setActionsOpen,
  actionModuleFilter,
  setActionModuleFilter,
  envName,
}: AuditPageDialogsProps) {
  const { t } = useTranslation('common');

  return (
    <>
      <Dialog open={!!selected} onOpenChange={(isOpen) => !isOpen && setSelected(null)}>
        <DialogContent className="max-w-2xl p-0">
          <DialogHeader>
            <DialogTitle>{t('audit.detailsTitle')}</DialogTitle>
            <DialogDescription>
              {selected?.action} — {selected?.entityName ?? selected?.entityType ?? ''}
            </DialogDescription>
          </DialogHeader>
          {selected && (
            <DialogBody className="p-0">
              <AuditEntryDetails entry={selected} envName={envName} />
            </DialogBody>
          )}
        </DialogContent>
      </Dialog>

      <AuditActionsDialog
        open={actionsOpen}
        onOpenChange={setActionsOpen}
        selected={filters.actionFilters}
        onToggle={filters.toggleAction}
        onClear={filters.clearActions}
        moduleFilter={actionModuleFilter}
        onModuleFilterChange={setActionModuleFilter}
      />

      <AuditExportDialog
        open={exportOpen}
        onOpenChange={setExportOpen}
        action={filters.exportAction}
        entityType={filters.entityTypeFilter}
        range={filters.resolvedRange}
        search={filters.query}
        mode={filters.mode}
        actor={filters.actor}
        severities={filters.severities}
      />
    </>
  );
}
