// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCopy, IconServerCog, IconTerminal2, IconX } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { copyToClipboard } from '@resources/utils/clipboard';
import { PERM } from '@core/auth/permissionKeys';
import type { BackupEncryptionKeyResponse } from '@resources/hooks/useStorageLocations';

type BackupKeyRevealPanelProps = {
  /** The key material as returned by the generate call. Shown once, never stored. */
  backupKey: BackupEncryptionKeyResponse;
  /** McHarbor runs directly on its host: the key is a file, not a Docker secret. */
  native?: boolean;
  installing: boolean;
  onInstall: () => void;
  onDismiss: () => void;
};

/**
 * The one-time reveal of a freshly generated backup key: the warning, the
 * key itself, the host setup command, and the two ways out of it (copy it
 * yourself, or let McHarbor install it as the Docker secret and restart).
 *
 * Clipboard writes can be refused by the browser, so every copy falls back
 * to a manual-copy dialog rather than failing silently — losing this value
 * means losing the ability to read back existing encrypted archives.
 */
export function BackupKeyRevealPanel({
  backupKey,
  native,
  installing,
  onInstall,
  onDismiss,
}: BackupKeyRevealPanelProps) {
  const { t } = useTranslation('settings');
  const [manualCopy, setManualCopy] = useState<{ value: string; label: string } | null>(null);

  async function copyValue(value: string, label: string, successMessage: string) {
    const result = await copyToClipboard(value);
    if (result.ok) {
      toast.success(successMessage);
    } else {
      setManualCopy({ value, label });
    }
  }

  return (
    <div className="mt-4 rounded-lg border border-border bg-background p-3">
      <div className="space-y-3">
        <div className="flex gap-3 rounded-md border border-warning/30 bg-warning/10 p-3 text-warning">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
          <p className="text-xs leading-5">{t('backupKey.generatedWarning')}</p>
        </div>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
          <code className="min-w-0 flex-1 rounded-md bg-muted px-3 py-2 font-mono text-xs break-all text-foreground">
            {backupKey.key}
          </code>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => copyValue(backupKey.key, t('backupKey.title'), t('toast.backupKeyCopied'))}
            >
              <IconCopy className="size-4" />
              {t('backupKey.copy')}
            </Button>
            <Button
              permission={PERM.SETTINGS_MANAGE}
              variant="outline"
              size="sm"
              onClick={onInstall}
              disabled={installing}
            >
              <IconServerCog className="size-4" />
              {installing ? t('backupKey.installing') : t('backupKey.install')}
            </Button>
            <Button variant="ghost" size="icon" aria-label={t('backupKey.clear')} onClick={onDismiss}>
              <IconX className="size-4" />
            </Button>
          </div>
        </div>
        <div className="rounded-md border border-border bg-muted/40 p-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-xs font-medium text-foreground">{t('backupKey.setupTitle')}</p>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                copyValue(backupKey.setupCommand, t('backupKey.setupTitle'), t('toast.backupKeySetupCopied'))
              }
            >
              <IconTerminal2 className="size-4" />
              {t('backupKey.copySetupCommand')}
            </Button>
          </div>
          <code className="block font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground">
            {backupKey.setupCommand}
          </code>
        </div>
        <p className="text-xs text-muted-foreground">
          {native ? t('backupKey.native.copyOnceHint') : t('backupKey.copyOnceHint')}
        </p>
      </div>

      <ManualCopyDialog
        open={manualCopy !== null}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setManualCopy(null);
        }}
        value={manualCopy?.value ?? ''}
        title={
          manualCopy?.label ? `${t('clipboard.manualCopyTitle', { ns: 'common' })}: ${manualCopy.label}` : undefined
        }
      />
    </div>
  );
}
