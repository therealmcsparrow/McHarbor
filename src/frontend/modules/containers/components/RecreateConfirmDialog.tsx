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
import { SecretFileModeToggle } from '@resources/components/secrets/SecretFileModeToggle';
import { PERM } from '@core/auth/permissionKeys';

type RecreateConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  changedFields: string[];
  loading: boolean;
  error?: string | null;
  secretFileModeOffered: boolean;
  secretFileMode: boolean;
  onSecretFileModeChange: (value: boolean) => void;
  onConfirm: () => void;
};

export function RecreateConfirmDialog({
  open: isOpen,
  onOpenChange,
  changedFields,
  loading,
  error,
  secretFileModeOffered,
  secretFileMode,
  onSecretFileModeChange,
  onConfirm,
}: RecreateConfirmDialogProps) {
  const { t } = useTranslation('containers');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('confirm.recreateTitle')}</DialogTitle>
          <DialogDescription>
            {changedFields.length > 0 ? t('confirm.recreateDescription') : t('confirm.recreateNowDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 p-3">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
          <div className="text-xs text-warning">{t('confirm.recreateWarning')}</div>
        </div>

        {changedFields.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">{t('confirm.changedFields')}</p>
            <div className="flex flex-wrap gap-1.5">
              {changedFields.map((field) => (
                <span key={field} className="rounded-md bg-muted px-2 py-0.5 font-mono text-xs text-foreground">
                  {field}
                </span>
              ))}
            </div>
          </div>
        )}

        {secretFileModeOffered && (
          <div className="border-t border-border pt-4">
            <SecretFileModeToggle
              checked={secretFileMode}
              onCheckedChange={onSecretFileModeChange}
              disabled={loading}
            />
          </div>
        )}

        {error && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {error}
          </p>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            {t('edit.cancelChanges')}
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={loading} permission={PERM.CONTAINERS_MANAGE}>
            {loading ? t('edit.saving') : t('confirm.recreateConfirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
