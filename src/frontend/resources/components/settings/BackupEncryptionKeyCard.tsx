// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCircleCheck, IconKey, IconPencil } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { cn } from '@resources/utils/cn';
import { PERM } from '@core/auth/permissionKeys';
import { BackupKeyInstallDialog } from '@resources/components/settings/BackupKeyInstallDialog';
import { BackupKeyRevealPanel } from '@resources/components/settings/BackupKeyRevealPanel';
import {
  type BackupEncryptionKeyResponse,
  useBackupEncryptionKeyStatus,
  useGenerateBackupEncryptionKey,
  useInstallBackupEncryptionKey,
} from '@resources/hooks/useStorageLocations';

/**
 * The backup encryption key: one AES-256 master key per McHarbor instance,
 * held as the `mcharbor_backup_key` Docker secret and used to wrap every
 * backup archive McHarbor writes.
 *
 * It lives on the Backups page rather than under Settings → Storage: the key
 * applies to the archive, not to the destination it is uploaded to, and the
 * people who need it are the ones running or restoring a backup. Settings →
 * Storage keeps a read-only status row that links here.
 *
 * The component stays in the settings module because its hooks do, and
 * because both routes are gated on `settings.view` / `settings.manage`.
 */
export function BackupEncryptionKeyCard() {
  const { t } = useTranslation('settings');
  const { data: currentStatus, isLoading: statusLoading } = useBackupEncryptionKeyStatus();
  // On a native install the key is a file under the data directory, not a Docker secret.
  const native = currentStatus?.native === true;
  const generateBackupKey = useGenerateBackupEncryptionKey();
  const installBackupKey = useInstallBackupEncryptionKey();
  const [revealed, setRevealed] = useState<BackupEncryptionKeyResponse | null>(null);
  const [regenerateOpen, setRegenerateOpen] = useState(false);
  const [installOpen, setInstallOpen] = useState(false);

  const active = currentStatus?.readable ?? false;
  // "Visible" covers both an installed key and one generated in this session
  // but not yet installed — either way, generating again replaces something.
  const visible = active || !!revealed;

  function generate() {
    generateBackupKey.mutate(undefined, { onSuccess: (data) => setRevealed(data) });
  }

  function handleGenerateClick() {
    if (visible) {
      setRegenerateOpen(true);
      return;
    }
    generate();
  }

  function handleConfirmRegenerate() {
    setRegenerateOpen(false);
    generate();
  }

  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex gap-3">
          <div
            className={cn(
              'flex size-10 shrink-0 items-center justify-center rounded-lg border transition-colors',
              visible ? 'border-chart-1/30 bg-chart-1/10 text-chart-1' : 'border-border bg-muted text-muted-foreground',
            )}
          >
            {visible ? <IconCircleCheck className="size-5" /> : <IconKey className="size-5" />}
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-medium text-foreground">{t('backupKey.title')}</h3>
              {active && (
                <Badge variant="success" className="py-1">
                  <IconCircleCheck className="size-3.5" />
                  {t('backupKey.activeStatus')}
                </Badge>
              )}
              {!active && revealed && (
                <Badge variant="success" className="py-1">
                  <IconCircleCheck className="size-3.5" />
                  {t('backupKey.generatedStatus')}
                </Badge>
              )}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {native ? t('backupKey.native.description') : t('backupKey.description')}
            </p>
            {active && currentStatus?.keyId && (
              <p className="mt-1 font-mono text-xs text-muted-foreground">
                {t('backupKey.fingerprint', { value: currentStatus.keyId })}
              </p>
            )}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            onClick={handleGenerateClick}
            disabled={generateBackupKey.isPending || statusLoading}
          >
            <IconKey className="size-4" />
            {generateBackupKey.isPending
              ? t('backupKey.generating')
              : visible
                ? t('backupKey.regenerate')
                : t('backupKey.generate')}
          </Button>
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            onClick={() => setInstallOpen(true)}
            disabled={installBackupKey.isPending}
          >
            <IconPencil className="size-4" />
            {t('backupKey.enter')}
          </Button>
        </div>
      </div>

      {revealed && (
        <BackupKeyRevealPanel
          backupKey={revealed}
          native={native}
          installing={installBackupKey.isPending}
          onInstall={() => installBackupKey.mutate({ key: revealed.key })}
          onDismiss={() => setRevealed(null)}
        />
      )}

      <ConfirmDialog
        open={regenerateOpen}
        onOpenChange={setRegenerateOpen}
        title={t('backupKey.regenerateTitle')}
        description={t('backupKey.regenerateDescription')}
        confirmLabel={t('backupKey.regenerate')}
        loading={generateBackupKey.isPending}
        onConfirm={handleConfirmRegenerate}
      />
      <BackupKeyInstallDialog
        open={installOpen}
        native={native}
        onOpenChange={setInstallOpen}
        onInstalled={() => setRevealed(null)}
      />
    </div>
  );
}
