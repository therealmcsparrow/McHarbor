// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileSearch } from '@tabler/icons-react';
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
import { formatBytes } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import {
  useStageUploadedContainerBackup,
  type ContainerBackupRun,
  type UploadProgress,
} from '@resources/hooks/useContainerBackups';

type UploadRestoreDialogProps = {
  containerId: string;
  dialogOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
  // Receives the staged run and the key it was read with; the caller opens the
  // regular restore dialog on it, which is where the selection happens.
  onStaged: (run: ContainerBackupRun, secretKey: string) => void;
};

export function UploadRestoreDialog({ containerId, dialogOpen, onOpenChange, onStaged }: UploadRestoreDialogProps) {
  const { t } = useTranslation('containers');
  const fileInputId = useId();
  const keyInputId = useId();
  const stageUpload = useStageUploadedContainerBackup(containerId);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadKey, setUploadKey] = useState('');
  const [progress, setProgress] = useState<UploadProgress | null>(null);

  function closeDialog() {
    onOpenChange(false);
    setUploadFile(null);
    setUploadKey('');
    setProgress(null);
  }

  function readUploadedBackup() {
    if (!uploadFile) return;
    const key = uploadKey.trim();
    stageUpload.mutate(
      { file: uploadFile, secretKey: key, onProgress: setProgress },
      {
        onSuccess: (run) => {
          closeDialog();
          onStaged(run, key);
        },
        onSettled: () => setProgress(null),
      },
    );
  }

  return (
    <Dialog
      open={dialogOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen && !stageUpload.isPending) closeDialog();
        if (isOpen) onOpenChange(true);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('backups.restoreUploadTitle')}</DialogTitle>
          <DialogDescription>{t('backups.restoreUploadDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div>
            <label htmlFor={fileInputId} className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('backups.restoreUploadFile')}
            </label>
            <input
              id={fileInputId}
              type="file"
              accept=".tar,.mcharbor.tar,application/x-tar"
              onChange={(changeEvent) => setUploadFile(changeEvent.target.files?.[0] ?? null)}
              className="mc-control block w-full rounded-md border border-input bg-background px-3 text-sm text-foreground file:mr-3 file:rounded-md file:border-0 file:bg-secondary file:px-3 file:py-0.5 file:text-xs file:font-medium file:text-secondary-foreground hover:file:bg-secondary/80 focus:ring-2 focus:ring-ring focus:outline-none"
            />
          </div>
          <div>
            <label htmlFor={keyInputId} className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('backups.restoreUploadKey')}
            </label>
            <Input
              id={keyInputId}
              type="password"
              value={uploadKey}
              onChange={(changeEvent) => setUploadKey(changeEvent.target.value)}
              placeholder={t('backups.restoreUploadKeyPlaceholder')}
              autoComplete="off"
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('backups.restoreUploadKeyHint')}</p>
          </div>
          {stageUpload.isPending && <UploadProgressBar progress={progress} />}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={closeDialog} disabled={stageUpload.isPending}>
            {t('backups.cancelRestore')}
          </Button>
          <Button
            onClick={readUploadedBackup}
            permission={PERM.RESTORE_RUN}
            disabled={stageUpload.isPending || !uploadFile}
            data-testid="upload-restore-read"
          >
            <IconFileSearch className="size-4" />
            {stageUpload.isPending ? t('backups.restoreUploadReading') : t('backups.restoreUploadRead')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function UploadProgressBar({ progress }: { progress: UploadProgress | null }) {
  const { t } = useTranslation('containers');
  const checking = progress?.phase === 'checking';
  const percent =
    progress && progress.total > 0 ? Math.min(100, Math.round((progress.loaded / progress.total) * 100)) : 0;
  return (
    <div className="space-y-1.5" data-testid="upload-restore-progress">
      <progress
        className={`h-1.5 w-full overflow-hidden rounded-full bg-muted [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-primary ${
          checking ? 'animate-pulse' : ''
        }`}
        value={checking ? undefined : percent}
        max={100}
        aria-label={t('backups.fileUpload.label')}
      />
      <p className="text-xs text-muted-foreground">
        {checking
          ? t('backups.fileUpload.checking')
          : progress
            ? t('backups.fileUpload.uploading', {
                percent,
                loaded: formatBytes(progress.loaded),
                total: formatBytes(progress.total),
              })
            : t('backups.fileUpload.starting')}
      </p>
    </div>
  );
}
