// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { VM_PERM } from '../permissions';
import { useCloneProgress, useCloneVM } from '../hooks/useVMs';
import type { VM } from '../types';

/**
 * Copy a stopped guest.
 *
 * The request stays open while the disks copy, and the copy's own row reports
 * how far it has got, so the dialog polls that row rather than guessing.
 */
export function CloneVMDialog({
  vm,
  onClose,
  onCloned,
}: {
  vm: VM;
  onClose: () => void;
  onCloned?: (id: string) => void;
}) {
  const { t } = useTranslation('vm');
  const clone = useCloneVM();
  const [cloneName, setCloneName] = useState(`${vm.name}-clone`);
  const [linked, setLinked] = useState(false);
  // The pending flag reaches the button a render late; a second click inside
  // that window must not start a second copy.
  const sending = useRef(false);
  const progress = useCloneProgress(vm.environmentId, cloneName.trim(), clone.isPending).data;

  const submit = () => {
    if (sending.current || !cloneName.trim()) return;
    sending.current = true;
    clone.mutate(
      { id: vm.id, name: cloneName.trim(), linked },
      {
        onSuccess: (created) => {
          onClose();
          onCloned?.(created.id);
        },
        onSettled: () => {
          sending.current = false;
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && !clone.isPending && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('clone.title', { name: vm.name })}</DialogTitle>
          <DialogDescription>{t('clone.identityNote')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="vm-clone-name">{t('clone.name')}</Label>
            <Input
              id="vm-clone-name"
              value={cloneName}
              disabled={clone.isPending}
              onChange={(evt) => setCloneName(evt.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('clone.mode.label')}</Label>
            <fieldset aria-label={t('clone.mode.label')} className="m-0 grid min-w-0 gap-2 border-0 p-0 sm:grid-cols-2">
              {([false, true] as const).map((option) => (
                <Button
                  key={String(option)}
                  variant="outline"
                  aria-pressed={linked === option}
                  disabled={clone.isPending}
                  onClick={() => setLinked(option)}
                  className={`h-auto flex-col items-start gap-1 p-3 text-left whitespace-normal ${
                    linked === option ? 'border-primary bg-primary/5' : ''
                  }`}
                >
                  <span className="text-sm font-medium text-foreground">
                    {option ? t('clone.mode.linked') : t('clone.mode.full')}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {option ? t('clone.linkedHint') : t('clone.fullHint')}
                  </span>
                </Button>
              ))}
            </fieldset>
          </div>
        </div>

        {clone.isPending && (
          <div className="space-y-2 rounded-md border border-border p-3">
            <div className="flex items-center gap-3">
              <Spinner className="h-4 w-4" />
              <p className="text-sm text-foreground">
                {progress ? t('clone.runningPercent', { percent: progress.percent }) : t('clone.running')}
              </p>
            </div>
            {progress && <ProgressBar value={progress.percent} />}
          </div>
        )}

        {clone.isError && <Alert variant="destructive" description={clone.error.message} />}

        <DialogFooter>
          <Button variant="outline" disabled={clone.isPending} onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={clone.isPending || !cloneName.trim()} onClick={submit}>
            {clone.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('clone.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
