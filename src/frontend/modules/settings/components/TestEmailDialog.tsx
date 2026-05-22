// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useTestEmailServer } from '../hooks/useEmailServers';

type TestEmailDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  serverId: string;
  serverName: string;
};

export function TestEmailDialog({ open, onOpenChange, serverId, serverName }: TestEmailDialogProps) {
  const { t } = useTranslation('settings');
  const [to, setTo] = useState('');
  const testEmail = useTestEmailServer();

  function handleOpenChange(value: boolean) {
    if (!value) setTo('');
    onOpenChange(value);
  }

  function handleSend() {
    testEmail.mutate(
      { id: serverId, to },
      { onSuccess: () => handleOpenChange(false) },
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('email.testDialogTitle')}</DialogTitle>
          <DialogDescription>
            {t('email.testDialogDescription', { name: serverName })}
          </DialogDescription>
        </DialogHeader>

        <div className="px-4 py-3">
          <Label htmlFor="test-to" className="mb-1">{t('email.testToLabel')}</Label>
          <Input
            variant="outline"
            id="test-to"
            type="email"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            placeholder={t('email.testToPlaceholder')}
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t('common:cancel', 'Cancel')}
          </Button>
          <Button
            onClick={handleSend}
            disabled={!to || testEmail.isPending}
          >
            {testEmail.isPending ? t('email.sending') : t('email.sendTest')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
