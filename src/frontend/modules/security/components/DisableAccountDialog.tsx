// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Textarea } from '@resources/components/ui/Textarea';
import { useDisableAccount } from '../hooks/useAccountManagement';
import { PERM } from '@core/auth/permissionKeys';

type DisableAccountDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  userId: string;
  username: string;
};

/**
 * DisableAccountDialog disables a user account, revokes all their
 * sessions and stores a reason in the audit log. The reason is
 * optional but encouraged — admins can type a brief explanation
 * that surfaces in the user detail view and audit log.
 */
export function DisableAccountDialog({ open: isOpen, onOpenChange, userId, username }: DisableAccountDialogProps) {
  const { t } = useTranslation('security');
  const disable = useDisableAccount();
  const [reason, setReason] = useState('');

  function handleSubmit() {
    disable.mutate({ userId, reason }, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('disableAccount.title')}</DialogTitle>
          <DialogDescription>
            {t('disableAccount.description', {
              username,
            })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-2">
          <label className="text-sm font-medium text-foreground">{t('disableAccount.reasonLabel')}</label>
          <Textarea
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={t('disableAccount.reasonPlaceholder')}
            rows={3}
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.ACCOUNT_MANAGE}
            variant="destructive"
            onClick={handleSubmit}
            disabled={disable.isPending}
          >
            {disable.isPending ? t('actions.processing', { ns: 'common' }) : t('actions.disable', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
