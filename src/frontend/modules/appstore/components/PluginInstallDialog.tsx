// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import type { PluginStreamEvent, PluginStreamLogEntry } from '../hooks/useStreamPlugin';
import type { PluginEntry, PluginVersion } from '../types/plugins';
import { PERM } from '@core/auth/permissionKeys';
import { PluginInstallProgress } from './PluginInstallProgress';
import { PluginVersionPicker } from './PluginVersionPicker';
import { isUnsignedVersion, PluginUnsignedWarning } from './PluginSignature';

interface PluginInstallDialogProps {
  title: string;
  description?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  progress: PluginStreamEvent | null;
  logs: PluginStreamLogEntry[];
  running: boolean;
  onAbort?: () => void;
  onClose: () => void;
  /**
   * Pre-install picker phase. When set, the dialog renders a version
   * selector + Install button instead of the progress UI. The picker
   * only makes sense for install (not update) — the host always picks
   * the latest catalog entry by slug for updates. Hidden when the
   * plugin has a single version (no point showing a 1-option dropdown),
   * in which case the parent should leave `pendingInstall` undefined
   * and let the SSE start immediately.
   */
  pendingInstall?: {
    entry: PluginEntry;
    versions: PluginVersion[];
  };
  selectedVersion?: PluginVersion | null;
  onSelectedVersionChange?: (version: PluginVersion) => void;
  onConfirmInstall?: () => void;
  pluginName?: string;
  unsignedRefused?: boolean;
  onConfirmUnsigned?: () => void;
}

export function PluginInstallDialog({
  title,
  description,
  open: isOpen,
  onOpenChange,
  progress,
  logs,
  running,
  onAbort,
  onClose,
  pendingInstall,
  selectedVersion,
  onSelectedVersionChange,
  onConfirmInstall,
  pluginName,
  unsignedRefused = false,
  onConfirmUnsigned,
}: PluginInstallDialogProps) {
  const { t } = useTranslation(['common', 'plugins']);
  const [riskAcknowledged, setRiskAcknowledged] = useState(false);

  const statusValue = progress?.status;
  const terminal = statusValue === 'done' || statusValue === 'error';

  // The release the Install button would actually fetch. The picker
  // defaults to versions[0], so fall back to it rather than treating a
  // not-yet-touched selector as "nothing chosen".
  const targetVersion = selectedVersion ?? pendingInstall?.versions[0] ?? null;
  const refused = unsignedRefused && !running;
  const unsigned = refused || isUnsignedVersion(targetVersion);

  // Re-arm the acknowledgement whenever the dialog closes or the chosen
  // release changes: consent given for one tarball is not consent for
  // another.
  const consentKey = `${isOpen}|${targetVersion?.download_url ?? ''}|${refused}`;
  const [armedFor, setArmedFor] = useState(consentKey);
  if (consentKey !== armedFor) {
    setArmedFor(consentKey);
    setRiskAcknowledged(false);
  }

  // Picker phase only renders for installs with >1 version. Single-
  // version plugins skip the picker entirely — the parent leaves
  // `pendingInstall` undefined and goes straight to SSE start, so this
  // dialog opens in progress mode.
  // Unsigned single-version plugins get the picker phase too — not for
  // the (pointless) one-option dropdown, but because it is the only
  // place the risk warning and its acknowledgement can be shown before
  // the install starts.
  const pickerMode =
    refused || (!!pendingInstall && (pendingInstall.versions.length > 1 || unsigned) && !running && !terminal);

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => {
        // Don't allow backdrop / Esc to close the dialog mid-install.
        // The progress UI uses onAbort instead.
        if (running && !next) return;
        onOpenChange(next);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>

        {pickerMode ? (
          <div className="space-y-3">
            {pendingInstall && pendingInstall.versions.length > 1 && (
              <PluginVersionPicker
                entry={pendingInstall.entry}
                versions={pendingInstall.versions}
                selectedVersion={selectedVersion}
                onSelectedVersionChange={onSelectedVersionChange}
              />
            )}
            {unsigned && (
              <PluginUnsignedWarning
                acknowledged={riskAcknowledged}
                onAcknowledgedChange={setRiskAcknowledged}
                pluginName={pluginName}
                refusedByServer={refused}
              />
            )}
          </div>
        ) : (
          <PluginInstallProgress progress={progress} logs={logs} />
        )}

        <DialogFooter>
          {pickerMode ? (
            <>
              <Button size="sm" variant="outline" onClick={onClose} aria-label={t('install.cancel', { ns: 'plugins' })}>
                {t('install.cancel', { ns: 'plugins' })}
              </Button>
              <Button
                permission={PERM.PLUGINS_MANAGE}
                size="sm"
                variant="default"
                onClick={refused ? onConfirmUnsigned : onConfirmInstall}
                disabled={(!refused && !targetVersion) || (unsigned && !riskAcknowledged)}
                aria-label={refused ? t('unsigned.installAnyway', { ns: 'plugins' }) : t('appStore.install')}
              >
                <IconDownload className="size-3.5" />
                {refused ? t('unsigned.installAnyway', { ns: 'plugins' }) : t('appStore.install')}
              </Button>
            </>
          ) : (
            <>
              {!terminal && onAbort && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={onAbort}
                  aria-label={t('install.cancel', { ns: 'plugins' })}
                >
                  {t('install.cancel', { ns: 'plugins' })}
                </Button>
              )}
              {terminal && (
                <Button size="sm" variant={statusValue === 'done' ? 'default' : 'outline'} onClick={onClose}>
                  {statusValue === 'done'
                    ? t('install.done', { ns: 'plugins' })
                    : t('install.close', { ns: 'plugins' })}
                </Button>
              )}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
