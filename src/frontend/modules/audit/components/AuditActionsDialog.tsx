// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The action-catalogue browser. Lifted out of AuditPage unchanged so the page
// itself is just the TablePage call plus its filter state.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { SEVERITY_VARIANTS } from '@resources/components/AuditEntryDetails';
import { useAuditActions } from '@resources/hooks/useAuditLogs';

const CHECK_MARK = 'x';

type AuditActionsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selected: string[];
  onToggle: (action: string) => void;
  onClear: () => void;
  moduleFilter: string;
  onModuleFilterChange: (value: string) => void;
};

export function AuditActionsDialog({
  open: isOpen,
  onOpenChange,
  selected,
  onToggle,
  onClear,
  moduleFilter,
  onModuleFilterChange,
}: AuditActionsDialogProps) {
  const { t } = useTranslation('common');
  const { data: actionCatalog = [] } = useAuditActions(moduleFilter);
  const [search, setSearch] = useState('');

  // A stale query from a previous visit would silently hide rows on reopen.
  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (!isOpen) setSearch('');
  }

  const moduleOptions = useMemo(() => {
    const modules = [...new Set(actionCatalog.map((entry) => entry.module))].sort();
    return [
      { value: '', label: t('audit.allModules') },
      ...modules.map((module) => ({ value: module, label: module })),
    ];
  }, [actionCatalog, t]);

  const visibleActions = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return actionCatalog;
    return actionCatalog.filter((entry) =>
      [entry.action, entry.module, entry.description].some((field) => field?.toLowerCase().includes(query)),
    );
  }, [actionCatalog, search]);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl p-0">
        <DialogHeader>
          <DialogTitle>{t('audit.actionBrowserTitle')}</DialogTitle>
          <DialogDescription>{t('audit.actionBrowserDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <div className="relative">
            <IconSearch className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('audit.searchActions')}
              aria-label={t('audit.searchActions')}
              className="pl-8"
            />
          </div>
          <div className="flex items-center justify-between gap-3">
            <Select
              value={moduleFilter}
              onChange={onModuleFilterChange}
              options={moduleOptions}
              placeholder={t('audit.filterModule')}
              className="w-44"
            />
            <Button variant="ghost" onClick={onClear}>
              {t('audit.clearActions')}
            </Button>
          </div>
          <div className="max-h-96 overflow-auto rounded-lg border border-border">
            {visibleActions.length === 0 ? (
              <div className="p-4 text-sm text-muted-foreground">
                {actionCatalog.length === 0 ? t('audit.noCatalogActions') : t('audit.noMatchingActions')}
              </div>
            ) : (
              <div className="divide-y divide-border">
                {visibleActions.map((entry) => {
                  const isSelected = selected.includes(entry.action);
                  return (
                    <Button
                      key={entry.id}
                      variant="ghost"
                      className="h-auto w-full justify-start rounded-none px-4 py-3 text-left"
                      onClick={() => onToggle(entry.action)}
                    >
                      <span className="flex size-4 shrink-0 items-center justify-center rounded border border-border text-xs">
                        {isSelected && CHECK_MARK}
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-1">
                        <span className="flex items-center gap-2">
                          <Badge variant={SEVERITY_VARIANTS[entry.severityDefault] ?? 'secondary'}>
                            {t(`audit.severity.${entry.severityDefault}`)}
                          </Badge>
                          <span className="font-mono text-xs text-muted-foreground">{entry.module}</span>
                          <span className="truncate font-medium text-foreground">{entry.action}</span>
                        </span>
                        <span className="truncate text-xs text-muted-foreground">{entry.description}</span>
                      </span>
                    </Button>
                  );
                })}
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={() => onOpenChange(false)}>{t('audit.applyActions')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
