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
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import type { UseMutationResult } from '@tanstack/react-query';

type CreateDesiredStateDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createState: UseMutationResult<unknown, Error, { name: string; containerName: string; imageRef: string; desiredStatus: string }>;
};

export function CreateDesiredStateDialog({ open, onOpenChange, createState }: CreateDesiredStateDialogProps) {
  const { t } = useTranslation('common');
  const [name, setName] = useState('');
  const [containerName, setContainerName] = useState('');
  const [imageRef, setImageRef] = useState('');
  const [desiredStatus, setDesiredStatus] = useState('running');

  const handleCreate = () => {
    if (!name.trim() || !containerName.trim() || !imageRef.trim()) return;
    createState.mutate(
      { name: name.trim(), containerName: containerName.trim(), imageRef: imageRef.trim(), desiredStatus },
      {
        onSuccess: () => {
          onOpenChange(false);
          setName('');
          setContainerName('');
          setImageRef('');
          setDesiredStatus('running');
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('reconciler.addTitle')}</DialogTitle>
          <DialogDescription>{t('reconciler.addDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('reconciler.nameLabel')}</Label>
            <Input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="my-service"
            />
          </div>
          <div>
            <Label className="mb-2">{t('reconciler.containerNameLabel')}</Label>
            <Input
              type="text"
              value={containerName}
              onChange={(e) => setContainerName(e.target.value)}
              placeholder="nginx-web"
            />
          </div>
          <div>
            <Label className="mb-2">{t('reconciler.imageRefLabel')}</Label>
            <Input
              type="text"
              value={imageRef}
              onChange={(e) => setImageRef(e.target.value)}
              placeholder="nginx:latest"
            />
          </div>
          <div>
            <Label className="mb-2">{t('reconciler.desiredStatusLabel')}</Label>
            <Select
              value={desiredStatus}
              onChange={setDesiredStatus}
              options={[
                { value: 'running', label: t('reconciler.statusRunning') },
                { value: 'stopped', label: t('reconciler.statusStopped') },
                { value: 'removed', label: t('reconciler.statusRemoved') },
              ]}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={createState.isPending || !name.trim() || !containerName.trim() || !imageRef.trim()}
          >
            {createState.isPending ? t('reconciler.creating') : t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

