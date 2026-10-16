// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import type { VM } from '../types';

export function CreateSnapshotDialog({
  vm,
  onClose,
  onSubmit,
}: {
  vm: VM;
  onClose: () => void;
  onSubmit: (input: { name: string; description?: string; withMemory?: boolean; pauseGuest?: boolean }) => void;
}) {
  const { t } = useTranslation('vm');

  const [nameValue, setName] = useState('');
  const [description, setDescription] = useState('');
  const [withMemory, setWithMemory] = useState(false);
  const [pauseGuest, setPauseGuest] = useState(true);

  // Said before the snapshot is taken, not discovered at restore time.
  const crashOnly = vm.guestAgentState !== 'connected';

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('snapshots.create.title')}</DialogTitle>
          <DialogDescription>{vm.name}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {crashOnly && (
            <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              <p className="text-sm text-foreground">{t('snapshots.create.crashWarning')}</p>
            </div>
          )}

          <div>
            <Label htmlFor="snapshot-name">{t('snapshots.create.name')}</Label>
            <Input
              id="snapshot-name"
              value={nameValue}
              onChange={(evt) => setName(evt.target.value)}
              autoComplete="off"
            />
          </div>

          <div>
            <Label htmlFor="snapshot-description">{t('snapshots.create.description')}</Label>
            <Textarea
              id="snapshot-description"
              value={description}
              onChange={(evt) => setDescription(evt.target.value)}
              rows={3}
              className="min-h-[5rem]"
            />
          </div>

          {crashOnly && vm.state === 'running' && (
            <div className="flex items-start justify-between gap-3">
              <div>
                <Label htmlFor="snapshot-pause" className="text-sm font-normal">
                  {t('snapshots.create.pauseGuest')}
                </Label>
                <p className="mt-0.5 text-xs text-muted-foreground">{t('snapshots.create.pauseGuestHint')}</p>
              </div>
              <Switch
                id="snapshot-pause"
                checked={pauseGuest || withMemory}
                disabled={withMemory}
                onCheckedChange={setPauseGuest}
              />
            </div>
          )}

          {vm.state === 'running' && (
            <div className="flex items-start justify-between gap-3">
              <div>
                <Label htmlFor="snapshot-memory" className="text-sm font-normal">
                  {t('snapshots.create.withMemory')}
                </Label>
                <p className="mt-0.5 text-xs text-muted-foreground">{t('snapshots.create.withMemoryHint')}</p>
              </div>
              <Switch id="snapshot-memory" checked={withMemory} onCheckedChange={setWithMemory} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={VM_PERM.MANAGE}
            disabled={!nameValue.trim()}
            onClick={() => onSubmit({ name: nameValue.trim(), description, withMemory, pauseGuest })}
          >
            {t('snapshots.create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
