// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { Label } from '@resources/components/ui/Label';
import { StatusError } from '@resources/components/ui/StatusError';
import { Hint } from '@resources/components/ui/Hint';
import { useFlash } from '@resources/hooks/useFlash';
import { copyToClipboard } from '@resources/utils/clipboard';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { cliDownloadUrl, cliLoginCommand, useCLIDownloads, type CLIArchive } from '../hooks/useCLIDownloads';

type CLIDownloadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const OS_KEY: Record<CLIArchive['os'], 'linux' | 'macos' | 'windows'> = {
  linux: 'linux',
  darwin: 'macos',
  windows: 'windows',
};

function formatSize(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function CLIDownloadDialog({ open: isOpen, onOpenChange }: CLIDownloadDialogProps) {
  const { t } = useTranslation('security');
  const downloads = useCLIDownloads(isOpen);
  const { flashing: copied, flash: flashCopied } = useFlash();
  const [manualOpen, setManualOpen] = useState(false);
  const [selfSigned, setSelfSigned] = useState(false);
  const command = cliLoginCommand(window.location.origin, selfSigned);
  const archives = downloads.data ?? [];

  const handleCopy = async () => {
    const result = await copyToClipboard(command);
    if (!result.ok) {
      setManualOpen(true);
      return;
    }
    flashCopied();
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('cli.title')}</DialogTitle>
          <DialogDescription>{t('cli.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t('cli.downloadTitle')}</h3>
            {downloads.isLoading ? (
              <Spinner size="sm" />
            ) : downloads.error ? (
              <div className="flex items-center gap-2 text-sm text-destructive">
                <span>{t('cli.loadFailed')}</span>
                <StatusError error={downloads.error.message} />
              </div>
            ) : archives.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('cli.none')}</p>
            ) : (
              <ul className="divide-y divide-border rounded-lg border border-border">
                {archives.map((archive) => (
                  <li key={archive.file} className="flex items-center gap-3 p-2">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium">
                        {t(`cli.os.${OS_KEY[archive.os]}`)} · {archive.arch}
                      </p>
                      <Hint text={archive.sha256}>
                        <p className="truncate font-mono text-xs text-muted-foreground">
                          {archive.file} · {formatSize(archive.size)}
                        </p>
                      </Hint>
                    </div>
                    <Button asChild variant="outline" size="sm">
                      <a
                        href={cliDownloadUrl(archive)}
                        download={archive.file}
                        aria-label={t('cli.downloadFor', { os: t(`cli.os.${OS_KEY[archive.os]}`), arch: archive.arch })}
                      >
                        <IconDownload className="size-4" />
                      </a>
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t('cli.loginTitle')}</h3>
            <p className="text-sm text-muted-foreground">{t('cli.loginHint')}</p>
            <div className="flex items-start gap-3 rounded-lg border border-border p-3">
              <Switch id="cli-self-signed" checked={selfSigned} onCheckedChange={setSelfSigned} />
              <div className="space-y-1">
                <Label htmlFor="cli-self-signed" className="cursor-pointer">
                  {t('cli.selfSigned')}
                </Label>
                <p className="text-xs text-muted-foreground">{t('cli.selfSignedHint')}</p>
              </div>
            </div>
            <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 p-3">
              <code className="flex-1 font-mono text-sm break-all">{command}</code>
              <Button variant="outline" size="sm" onClick={handleCopy} aria-label={t('cli.copyCommand')}>
                <IconCopy className="size-4" />
              </Button>
            </div>
            {copied && <p className="text-sm text-success">{t('cli.copied')}</p>}
          </section>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('actions.close', { ns: 'common' })}</Button>
        </DialogFooter>
      </DialogContent>

      <ManualCopyDialog open={manualOpen} onOpenChange={setManualOpen} value={command} />
    </Dialog>
  );
}
