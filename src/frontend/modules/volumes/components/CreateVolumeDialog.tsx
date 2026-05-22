// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import type { UseMutationResult } from '@tanstack/react-query';

type CreateVolumeDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createVolume: UseMutationResult<unknown, Error, { name: string; driver?: string }>;
};

export function CreateVolumeDialog({ open, onOpenChange, createVolume }: CreateVolumeDialogProps) {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const [volumeName, setVolumeName] = useState('');
  const [volumeDriver, setVolumeDriver] = useState('local');

  const handleCreate = () => {
    if (!volumeName.trim()) return;
    createVolume.mutate(
      { name: volumeName.trim(), driver: volumeDriver },
      {
        onSuccess: () => {
          onOpenChange(false);
          setVolumeName('');
          setVolumeDriver('local');
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            variant="outline"
            type="text"
            value={volumeName}
            onChange={(e) => setVolumeName(e.target.value)}
            placeholder={t('create.name')}
          />
          <Input
            variant="outline"
            type="text"
            value={volumeDriver}
            onChange={(e) => setVolumeDriver(e.target.value)}
            placeholder={t('create.driverPlaceholder')}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={handleCreate} disabled={createVolume.isPending || !volumeName.trim()}>
            {createVolume.isPending ? t('create.creating') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
