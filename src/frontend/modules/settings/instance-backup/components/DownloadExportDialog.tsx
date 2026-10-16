// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { Input } from '@resources/components/ui/Input';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { downloadURL, useDownloadStepUp, type InstanceExport } from '../hooks/useInstanceBackups';

type DownloadExportDialogProps = {
  target: InstanceExport | null;
  onOpenChange: (open: boolean) => void;
};

function startDownload(id: string) {
  const link = document.createElement('a');
  link.href = downloadURL(id);
  link.rel = 'noopener';
  link.click();
}

export function DownloadExportDialog({ target, onOpenChange }: DownloadExportDialogProps) {
  const { t } = useTranslation('instancebackup');
  const [password, setPassword] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const stepUp = useDownloadStepUp();

  const closeDialog = (next: boolean) => {
    if (!next) {
      setPassword('');
      setTotpCode('');
      stepUp.reset();
    }
    onOpenChange(next);
  };

  const submit = () => {
    if (!target) return;
    const id = target.id;
    stepUp.mutate(
      { password: password || undefined, totpCode: totpCode || undefined },
      {
        onSuccess: () => {
          startDownload(id);
          closeDialog(false);
        },
      },
    );
  };

  return (
    <Dialog open={target !== null} onOpenChange={closeDialog}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('download.title')}</DialogTitle>
          <DialogDescription>{t('download.description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <Alert variant="warning">{t('download.warning')}</Alert>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ib-dl-password">{t('download.password')}</Label>
            <PasswordInput
              id="ib-dl-password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ib-dl-totp">{t('download.totp')}</Label>
            <Input
              id="ib-dl-totp"
              inputMode="numeric"
              autoComplete="one-time-code"
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t('download.totpHint')}</p>
          </div>
          {stepUp.error && <Alert variant="destructive">{stepUp.error.message}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => closeDialog(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.INSTANCE_BACKUP_MANAGE}
            disabled={(password === '' && totpCode === '') || stepUp.isPending}
            onClick={submit}
          >
            {t('download.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
