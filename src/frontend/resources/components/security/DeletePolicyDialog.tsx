// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { PERM } from '@core/auth/permissionKeys';

type DeletePolicyDialogProps = {
  policyName: string | undefined;
  isOpen: boolean;
  isPending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
};

export function DeletePolicyDialog({ policyName, isOpen, isPending, onCancel, onConfirm }: DeletePolicyDialogProps) {
  const { t } = useTranslation('abac');
  return (
    <Dialog open={isOpen} onOpenChange={(next) => !next && onCancel()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('delete.title')}</DialogTitle>
          <DialogDescription>{t('delete.description', { name: policyName })}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <p className="text-sm text-muted-foreground">{t('delete.warning')}</p>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onCancel}>
            {t('delete.cancel')}
          </Button>
          <Button permission={PERM.RBAC_POLICIES_MANAGE} variant="destructive" onClick={onConfirm} disabled={isPending}>
            {t('delete.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
