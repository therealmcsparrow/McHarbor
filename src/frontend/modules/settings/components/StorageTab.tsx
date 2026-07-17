// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCircleCheck, IconCopy, IconDatabase, IconKey, IconPencil, IconPlus, IconServerCog, IconTerminal2, IconX } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
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
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { copyToClipboard } from '@resources/utils/clipboard';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { StorageLocationCard } from './StorageLocationCard';
import { StorageLocationDialog } from './StorageLocationDialog';
import {
  type StorageLocation,
  type BackupEncryptionKeyResponse,
  useBackupEncryptionKeyStatus,
  useGenerateBackupEncryptionKey,
  useInstallBackupEncryptionKey,
  useStorageLocations,
} from '../hooks/useStorageLocations';

export function StorageTab() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: locations = [], isLoading } = useStorageLocations();
  const { data: backupKeyStatus, isLoading: backupKeyStatusLoading } = useBackupEncryptionKeyStatus();
  const generateBackupKey = useGenerateBackupEncryptionKey();
  const installBackupKey = useInstallBackupEncryptionKey();
  const [createOpen, setCreateOpen] = useState(false);
  const [editLocation, setEditLocation] = useState<StorageLocation | null>(null);
  const [backupKey, setBackupKey] = useState<BackupEncryptionKeyResponse | null>(null);
  const [regenerateOpen, setRegenerateOpen] = useState(false);
  const [customKeyOpen, setCustomKeyOpen] = useState(false);
  const [customBackupKey, setCustomBackupKey] = useState('');
  const [customProjectPath, setCustomProjectPath] = useState('');
  const [showAdvancedInstall, setShowAdvancedInstall] = useState(false);
  const [manualCopy, setManualCopy] = useState<{ value: string; label: string } | null>(null);

  function handleGenerateBackupKey() {
    if (backupKey || backupKeyStatus?.readable) {
      setRegenerateOpen(true);
      return;
    }
    generateBackupKey.mutate(undefined, {
      onSuccess: (data) => setBackupKey(data),
    });
  }

  function handleConfirmRegenerateBackupKey() {
    setRegenerateOpen(false);
    generateBackupKey.mutate(undefined, {
      onSuccess: (data) => setBackupKey(data),
    });
  }

  async function handleCopyBackupKey() {
    if (!backupKey) return;
    const result = await copyToClipboard(backupKey.key);
    if (result.ok) {
      toast.success(t('toast.backupKeyCopied'));
    } else {
      setManualCopy({ value: backupKey.key, label: t('storage.backupKeyTitle') });
    }
  }

  async function handleCopyBackupSetupCommand() {
    if (!backupKey) return;
    const result = await copyToClipboard(backupKey.setupCommand);
    if (result.ok) {
      toast.success(t('toast.backupKeySetupCopied'));
    } else {
      setManualCopy({ value: backupKey.setupCommand, label: t('storage.backupKeySetupTitle') });
    }
  }

  function handleInstallBackupKey() {
    if (!backupKey) return;
    installBackupKey.mutate({ key: backupKey.key });
  }

  function handleInstallCustomBackupKey() {
    const key = customBackupKey.trim();
    const projectPath = customProjectPath.trim();
    installBackupKey.mutate(
      { key, projectPath: projectPath || undefined },
      {
        onSuccess: () => {
          setCustomBackupKey('');
          setCustomProjectPath('');
          setShowAdvancedInstall(false);
          setCustomKeyOpen(false);
          setBackupKey(null);
        },
      },
    );
  }

  function isValidCustomBackupKey() {
    try {
      const decoded = Uint8Array.from(atob(customBackupKey.trim()), (char) => char.charCodeAt(0));
      return decoded.length === 32;
    } catch {
      return false;
    }
  }

  const backupKeyActive = backupKeyStatus?.readable ?? false;
  const backupKeyVisible = backupKeyActive || !!backupKey;
  const customKeyValid = isValidCustomBackupKey();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex gap-3">
            <div
              className={cn(
                'flex size-10 shrink-0 items-center justify-center rounded-lg border transition-colors',
                backupKeyVisible
                  ? 'border-teal-500/30 bg-teal-500/10 text-teal-700 dark:text-teal-400'
                  : 'border-border bg-muted text-muted-foreground',
              )}
            >
              {backupKeyVisible ? <IconCircleCheck className="size-5" /> : <IconKey className="size-5" />}
            </div>
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-medium text-foreground">{t('storage.backupKeyTitle')}</h3>
                {backupKeyActive && (
                  <Badge variant="success" className="py-1">
                    <IconCircleCheck className="size-3.5" />
                    {t('storage.backupKeyActiveStatus')}
                  </Badge>
                )}
                {!backupKeyActive && backupKey && (
                  <Badge variant="success" className="py-1">
                    <IconCircleCheck className="size-3.5" />
                    {t('storage.backupKeyGeneratedStatus')}
                  </Badge>
                )}
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{t('storage.backupKeyDescription')}</p>
              {backupKeyActive && backupKeyStatus?.keyId && (
                <p className="mt-1 font-mono text-xs text-muted-foreground">
                  {t('storage.backupKeyFingerprint', { value: backupKeyStatus.keyId })}
                </p>
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={handleGenerateBackupKey}
              disabled={generateBackupKey.isPending || backupKeyStatusLoading}
            >
              <IconKey className="size-4" />
              {generateBackupKey.isPending
                ? t('storage.generatingBackupKey')
                : backupKeyVisible
                  ? t('storage.regenerateBackupKey')
                  : t('storage.generateBackupKey')}
            </Button>
            <Button
              variant="outline"
              onClick={() => setCustomKeyOpen(true)}
              disabled={installBackupKey.isPending}
            >
              <IconPencil className="size-4" />
              {t('storage.enterBackupKey')}
            </Button>
          </div>
        </div>

        {backupKey && (
          <div className="mt-4 rounded-lg border border-border bg-background p-3">
            <div className="space-y-3">
              <div className="flex gap-3 rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-yellow-800 dark:text-yellow-300">
                <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
                <p className="text-xs leading-5">{t('storage.backupKeyGeneratedWarning')}</p>
              </div>
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
                <code className="min-w-0 flex-1 break-all rounded-md bg-muted px-3 py-2 font-mono text-xs text-foreground">
                  {backupKey.key}
                </code>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button variant="outline" size="sm" onClick={handleCopyBackupKey}>
                    <IconCopy className="size-4" />
                    {t('storage.copyBackupKey')}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleInstallBackupKey}
                    disabled={installBackupKey.isPending}
                  >
                    <IconServerCog className="size-4" />
                    {installBackupKey.isPending ? t('storage.installingBackupKey') : t('storage.installBackupKey')}
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={t('storage.clearBackupKey')} onClick={() => setBackupKey(null)}>
                    <IconX className="size-4" />
                  </Button>
                </div>
              </div>
              <div className="rounded-md border border-border bg-muted/40 p-3">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <p className="text-xs font-medium text-foreground">{t('storage.backupKeySetupTitle')}</p>
                  <Button variant="outline" size="sm" onClick={handleCopyBackupSetupCommand}>
                    <IconTerminal2 className="size-4" />
                    {t('storage.copyBackupSetupCommand')}
                  </Button>
                </div>
                <code className="block whitespace-pre-wrap break-all font-mono text-xs text-muted-foreground">
                  {backupKey.setupCommand}
                </code>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('storage.backupKeyCopyOnceHint')}
              </p>
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-muted-foreground">{t('storage.description')}</p>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="size-4" />
          {t('storage.addLocation')}
        </Button>
      </div>

      {locations.length > 0 ? (
        <div className="space-y-3">
          {locations.map((location) => (
            <StorageLocationCard
              key={location.id}
              location={location}
              onEdit={setEditLocation}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
          <IconDatabase className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">{t('storage.noLocations')}</p>
        </div>
      )}

      <StorageLocationDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editLocation && (
        <StorageLocationDialog
          open={!!editLocation}
          onOpenChange={(open) => {
            if (!open) setEditLocation(null);
          }}
          location={editLocation}
        />
      )}
      <ConfirmDialog
        open={regenerateOpen}
        onOpenChange={setRegenerateOpen}
        title={t('storage.regenerateBackupKeyTitle')}
        description={t('storage.regenerateBackupKeyDescription')}
        confirmLabel={t('storage.regenerateBackupKey')}
        loading={generateBackupKey.isPending}
        onConfirm={handleConfirmRegenerateBackupKey}
      />
      <Dialog
        open={customKeyOpen}
        onOpenChange={(open) => {
          setCustomKeyOpen(open);
          if (!open) setCustomBackupKey('');
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('storage.enterBackupKeyTitle')}</DialogTitle>
            <DialogDescription>{t('storage.enterBackupKeyDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            <Input
              type="password"
              value={customBackupKey}
              onChange={(event) => setCustomBackupKey(event.target.value)}
              placeholder={t('storage.enterBackupKeyPlaceholder')}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">{t('storage.enterBackupKeyHint')}</p>
            <button
              type="button"
              className="text-xs text-muted-foreground underline"
              onClick={() => setShowAdvancedInstall((v) => !v)}
            >
              {showAdvancedInstall ? t('storage.installAdvancedHide') : t('storage.installAdvancedShow')}
            </button>
            {showAdvancedInstall && (
              <div className="rounded-md border border-border p-3">
                <label className="mb-1 block text-xs font-medium text-foreground">
                  {t('storage.installProjectPathLabel')}
                </label>
                <Input
                  value={customProjectPath}
                  onChange={(event) => setCustomProjectPath(event.target.value)}
                  placeholder={t('storage.installProjectPathPlaceholder')}
                  autoComplete="off"
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('storage.installProjectPathHint')}
                </p>
              </div>
            )}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setCustomKeyOpen(false);
                setCustomBackupKey('');
                setCustomProjectPath('');
                setShowAdvancedInstall(false);
              }}
            >
              {t('storage.cancelBackupKeyEntry')}
            </Button>
            <Button
              onClick={handleInstallCustomBackupKey}
              disabled={installBackupKey.isPending || !customKeyValid}
            >
              <IconServerCog className="size-4" />
              {installBackupKey.isPending ? t('storage.installingBackupKey') : t('storage.installEnteredBackupKey')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ManualCopyDialog
        open={manualCopy !== null}
        onOpenChange={(open) => {
          if (!open) setManualCopy(null);
        }}
        value={manualCopy?.value ?? ''}
        title={manualCopy?.label ? `${tc('clipboard.manualCopyTitle')}: ${manualCopy.label}` : undefined}
      />
    </div>
  );
}
