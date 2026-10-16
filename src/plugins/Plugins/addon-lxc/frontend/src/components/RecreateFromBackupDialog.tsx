// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useBackupManifest } from '../hooks/useRecreate';
import type { BackupRun } from '../hooks/useCTBackups';
import { formatAbsolute } from '../format';
import { RecreateForm } from './RecreateForm';

export function RecreateFromBackupDialog({
  run,
  environments,
  onOpenChange,
  onCreated,
}: {
  run: BackupRun;
  environments?: { id: string; name: string }[];
  onOpenChange: (open: boolean) => void;
  onCreated?: (containerId: string) => void;
}) {
  const { t } = useTranslation('ct');
  const manifest = useBackupManifest(run.id);
  const title = t('recreate.dialog.title', {
    name: manifest.data?.create.name || run.containerName || run.containerId,
  });

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {t('recreate.dialog.description', { taken: formatAbsolute(run.startedAt) })}
          </DialogDescription>
        </DialogHeader>

        {manifest.isLoading && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="h-4 w-4" />
            {t('recreate.dialog.loading')}
          </p>
        )}

        {manifest.isError && (
          <>
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {manifest.error.message}
            </p>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t('actions.cancel')}
              </Button>
            </DialogFooter>
          </>
        )}

        {manifest.data && (
          <RecreateForm
            draft={manifest.data}
            runId={run.id}
            environments={environments ?? []}
            onCancel={() => onOpenChange(false)}
            onCreated={(containerId) => {
              onOpenChange(false);
              onCreated?.(containerId);
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
