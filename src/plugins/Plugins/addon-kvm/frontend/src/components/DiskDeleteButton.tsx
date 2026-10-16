// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useDeleteDisk } from '../hooks/useVMs';
import type { Disk, VM } from '../types';

interface DiskDeleteButtonProps {
  vm: VM;
  disk: Disk;
  /** The guest takes the change now: stopped, or running with hot-plug. */
  changeable: boolean;
  /** The guest is running, so the boot disk is off limits. */
  live: boolean;
  bootDisk: boolean;
}

export function DiskDeleteButton({ vm, disk, changeable, live, bootDisk }: DiskDeleteButtonProps) {
  const { t } = useTranslation('vm');
  const deleteDisk = useDeleteDisk();
  const [confirming, setConfirming] = useState(false);

  // The boot disk goes with the guest, never on its own, and never from under
  // a running one.
  const bootLocked = live && bootDisk;
  const blocked = !changeable || disk.isBoot || bootLocked;

  const button = (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t('actions.delete')}
      permission={VM_PERM.DELETE}
      disabled={blocked}
      onClick={() => {
        deleteDisk.reset();
        setConfirming(true);
      }}
    >
      <IconTrash className="h-4 w-4" aria-hidden="true" />
    </Button>
  );

  return (
    <>
      {bootLocked ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span>{button}</span>
          </TooltipTrigger>
          <TooltipContent>{t('hardware.bootDiskRunning')}</TooltipContent>
        </Tooltip>
      ) : (
        button
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('hardware.deleteDisk.title')}
        description={t('hardware.deleteDisk.body')}
        confirmLabel={t('hardware.deleteDisk.confirm')}
        permission={VM_PERM.DELETE}
        loading={deleteDisk.isPending}
        onConfirm={() => {
          if (deleteDisk.isPending) return;
          deleteDisk.mutate({ id: vm.id, diskId: disk.id }, { onSuccess: () => setConfirming(false) });
        }}
      >
        {deleteDisk.isError && (
          <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {deleteDisk.error.message}
          </p>
        )}
      </ConfirmDialog>
    </>
  );
}
