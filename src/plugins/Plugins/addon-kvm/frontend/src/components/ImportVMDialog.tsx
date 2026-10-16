// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { IconMinimize } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { dismissImportJob, startImportConvert, startImportInspect } from '../importJob';
import { formatBytes } from '../format';
import { VM_PERM } from '../permissions';
import { nicAttachment } from '../utils/nicAttachment';
import { useImportForm } from '../hooks/useImportForm';
import { ImportReviewStep } from './ImportReviewStep';
import { ImportUploadStep } from './ImportUploadStep';

interface ImportVMDialogProps {
  environments: { id: string; name: string }[];
  onClose: () => void;
  onImported: (vmId: string) => void;
}

export function ImportVMDialog({ environments, onClose, onImported }: ImportVMDialogProps) {
  const { t } = useTranslation('vm');
  const queryClient = useQueryClient();
  const form = useImportForm(environments, onImported);
  const {
    job,
    running,
    uploaded,
    found,
    step,
    hostName,
    envId,
    nameValue,
    pool,
    network,
    vcpus,
    memoryMb,
    firmware,
    guestOs,
    secureBoot,
    tpmEnabled,
    autostart,
  } = form;

  const closeDialog = () => {
    if (!running) dismissImportJob();
    onClose();
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && closeDialog()}>
      <DialogContent
        className="max-w-2xl"
        onInteractOutside={(evt) => evt.preventDefault()}
        onEscapeKeyDown={(evt) => evt.preventDefault()}
        onPointerDownOutside={(evt) => evt.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('import.title')}</DialogTitle>
          <DialogDescription>{t('import.body')}</DialogDescription>
        </DialogHeader>

        {job.error ? <Alert variant="destructive" title={t('import.failed')} description={job.error} /> : null}

        {step === 'upload' ? <ImportUploadStep environments={environments} form={form} /> : null}

        {step === 'inspect' && uploaded ? (
          <div className="grid gap-3">
            <p className="text-sm text-foreground">
              {uploaded.fileName} · {uploaded.sourceKind} · {formatBytes(uploaded.sizeBytes)}
            </p>
            <p className="text-xs text-muted-foreground">{t('import.staged', { host: hostName })}</p>
            <Button
              permission={VM_PERM.IMPORT}
              disabled={!envId || running}
              onClick={() => startImportInspect(envId, queryClient)}
            >
              {t('import.inspect')}
            </Button>
            {job.phase === 'inspecting' ? (
              <p className="text-xs text-muted-foreground">{t('import.phase.inspecting')}</p>
            ) : null}
          </div>
        ) : null}

        {step === 'review' && uploaded && found ? <ImportReviewStep form={form} found={found} /> : null}

        <DialogFooter>
          {running ? (
            <Button variant="outline" onClick={onClose}>
              <IconMinimize className="mr-1.5 h-4 w-4" aria-hidden="true" />
              {t('import.background')}
            </Button>
          ) : (
            <Button variant="ghost" onClick={closeDialog}>
              {t('actions.cancel', { ns: 'common' })}
            </Button>
          )}
          {step === 'review' ? (
            <Button
              permission={VM_PERM.IMPORT}
              disabled={!nameValue.trim() || !pool || !network || running}
              onClick={() => {
                if (!pool || !network) return;
                startImportConvert(
                  {
                    environmentId: envId,
                    name: nameValue,
                    poolId: pool.id,
                    poolBackend: pool.backend,
                    poolTarget: pool.target,
                    networkId: network.id,
                    ...nicAttachment(network),
                    vcpus,
                    memoryMb,
                    firmware,
                    guestOsFamily: guestOs || 'linux',
                    secureBoot,
                    tpmEnabled,
                    autostart,
                  },
                  queryClient,
                );
              }}
            >
              {t('import.convert')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
