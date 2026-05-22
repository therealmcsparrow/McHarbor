// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';

type RecreateConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  changedFields: string[];
  loading: boolean;
  onConfirm: () => void;
};

export function RecreateConfirmDialog({
  open,
  onOpenChange,
  changedFields,
  loading,
  onConfirm,
}: RecreateConfirmDialogProps) {
  const { t } = useTranslation('containers');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('confirm.recreateTitle')}</DialogTitle>
          <DialogDescription>{t('confirm.recreateDescription')}</DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-yellow-500" />
          <div className="text-xs text-yellow-500">
            {t('confirm.recreateWarning')}
          </div>
        </div>

        {changedFields.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">{t('confirm.changedFields')}</p>
            <div className="flex flex-wrap gap-1.5">
              {changedFields.map((field) => (
                <span
                  key={field}
                  className="rounded-md bg-muted px-2 py-0.5 font-mono text-xs text-foreground"
                >
                  {field}
                </span>
              ))}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            {t('edit.cancelChanges')}
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={loading}>
            {loading ? t('edit.saving') : t('confirm.recreateConfirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
