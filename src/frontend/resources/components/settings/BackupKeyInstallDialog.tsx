// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconServerCog } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { PERM } from '@core/auth/permissionKeys';
import { useInstallBackupEncryptionKey } from '@resources/hooks/useStorageLocations';

type BackupKeyInstallDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called after a successful install so the caller can drop any revealed key. */
  onInstalled?: () => void;
  /** McHarbor runs directly on its host: there is no Compose project to point at. */
  native?: boolean;
};

/** A backup key is a base64-encoded 32-byte value; anything else is rejected. */
function isValidBackupKey(value: string) {
  try {
    const decoded = Uint8Array.from(atob(value.trim()), (char) => char.charCodeAt(0));
    return decoded.length === 32;
  } catch {
    return false;
  }
}

/**
 * Install an existing backup key — the path taken when restoring an archive
 * encrypted by a previous instance, or when the key is managed outside
 * McHarbor. The Compose project path is only needed when McHarbor was not
 * started by Compose, so it hides behind an "advanced" toggle.
 */
export function BackupKeyInstallDialog({
  open: isOpen,
  onOpenChange,
  onInstalled,
  native,
}: BackupKeyInstallDialogProps) {
  const { t } = useTranslation('settings');
  const installBackupKey = useInstallBackupEncryptionKey();
  const [key, setKey] = useState('');
  const [projectPath, setProjectPath] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);

  function reset() {
    setKey('');
    setProjectPath('');
    setShowAdvanced(false);
  }

  function handleClose() {
    reset();
    onOpenChange(false);
  }

  function handleInstall() {
    installBackupKey.mutate(
      { key: key.trim(), projectPath: projectPath.trim() || undefined },
      {
        onSuccess: () => {
          reset();
          onOpenChange(false);
          onInstalled?.();
        },
      },
    );
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('backupKey.enterTitle')}</DialogTitle>
          <DialogDescription>
            {native ? t('backupKey.native.enterDescription') : t('backupKey.enterDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <Input
            type="password"
            value={key}
            onChange={(changeEvent) => setKey(changeEvent.target.value)}
            placeholder={t('backupKey.enterPlaceholder')}
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">{t('backupKey.enterHint')}</p>
          {!native && (
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 text-xs"
              onClick={() => setShowAdvanced((value) => !value)}
            >
              {showAdvanced ? t('backupKey.installAdvancedHide') : t('backupKey.installAdvancedShow')}
            </Button>
          )}
          {!native && showAdvanced && (
            <div className="rounded-md border border-border p-3">
              <label className="mb-1 block text-xs font-medium text-foreground">
                {t('backupKey.installProjectPathLabel')}
              </label>
              <Input
                value={projectPath}
                onChange={(changeEvent) => setProjectPath(changeEvent.target.value)}
                placeholder={t('backupKey.installProjectPathPlaceholder')}
                autoComplete="off"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t('backupKey.installProjectPathHint')}</p>
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            {t('backupKey.cancelEntry')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            onClick={handleInstall}
            disabled={installBackupKey.isPending || !isValidBackupKey(key)}
          >
            <IconServerCog className="size-4" />
            {installBackupKey.isPending ? t('backupKey.installing') : t('backupKey.installEntered')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
