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
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { FileUpload } from '@resources/components/ui/FileUpload';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { InstanceBackupRefusal, useVerifyExport, type InstanceExport } from '../hooks/useInstanceBackups';

type VerifyExportDialogProps = {
  target: InstanceExport | null;
  onOpenChange: (open: boolean) => void;
};

export function VerifyExportDialog({ target, onOpenChange }: VerifyExportDialogProps) {
  const { t } = useTranslation('instancebackup');
  const [passphrase, setPassphrase] = useState('');
  const [keyFile, setKeyFile] = useState('');
  const verify = useVerifyExport();

  const closeDialog = (next: boolean) => {
    if (!next) {
      setPassphrase('');
      setKeyFile('');
      verify.reset();
    }
    onOpenChange(next);
  };

  const submit = () => {
    if (!target) return;
    verify.mutate(
      { id: target.id, ...(target.keyKind === 'keyfile' ? { keyFile } : { passphrase }) },
      { onSuccess: () => closeDialog(false) },
    );
  };

  const error = verify.error;
  const message = error instanceof InstanceBackupRefusal ? t(`restore.reasons.${error.reason}`) : error?.message;
  const ready = target?.keyKind === 'keyfile' ? keyFile !== '' : passphrase !== '';

  return (
    <Dialog open={target !== null} onOpenChange={closeDialog}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('verify.title')}</DialogTitle>
          <DialogDescription>{t('verify.description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          {target?.keyKind === 'keyfile' ? (
            <FileUpload
              accept=".key,.txt"
              label={t('restore.keyFile')}
              onChange={(files) => {
                const f = files[0];
                if (!f) return setKeyFile('');
                void f.text().then(setKeyFile);
              }}
            />
          ) : (
            <>
              <Label htmlFor="ib-verify-pass">{t('create.passphrase')}</Label>
              <PasswordInput
                id="ib-verify-pass"
                autoComplete="off"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
              />
            </>
          )}
        </div>
        {message && <Alert variant="destructive">{message}</Alert>}
        <DialogFooter>
          <Button variant="outline" onClick={() => closeDialog(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.INSTANCE_BACKUP_MANAGE} disabled={!ready || verify.isPending} onClick={submit}>
            {t('verify.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
