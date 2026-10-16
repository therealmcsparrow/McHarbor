// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';

type HttpsRegenerateDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

/**
 * Confirmation for replacing the self-signed certificate. Worth a prompt:
 * any client that pinned the current certificate has to trust the new one.
 */
export function HttpsRegenerateDialog({ open: isOpen, onOpenChange, onConfirm }: HttpsRegenerateDialogProps) {
  const { t } = useTranslation('security');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('https.regenerate.confirmTitle')}</DialogTitle>
          <DialogDescription>{t('https.regenerate.confirmBody')}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button permission={PERM.SETTINGS_MANAGE} onClick={onConfirm}>
            {t('https.regenerate.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
