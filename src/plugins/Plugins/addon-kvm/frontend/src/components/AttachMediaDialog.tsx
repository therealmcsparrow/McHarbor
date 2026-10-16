// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
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
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { VM_PERM } from '../permissions';
import { useAttachMedia } from '../hooks/useVMs';
import { useComputeImages } from '../hooks/useComputeResources';
import type { VM } from '../types';

export function AttachMediaDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const attach = useAttachMedia();
  const { data: discs } = useComputeImages(vm.environmentId, 'iso');

  const [pickedImageId, setImageId] = useState('');

  const imageId = pickedImageId || (discs?.length ? discs[0]!.id : '');

  const disc = discs?.find((entry) => entry.id === imageId);

  const submit = () => {
    if (!disc?.cache?.localPath) return;
    attach.mutate(
      {
        id: vm.id,
        imageId: disc.id,
        sourcePath: disc.cache.localPath,
        label: disc.displayName,
      },
      { onSuccess: onClose },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('hardware.media.attach.title')}</DialogTitle>
          <DialogDescription>
            {vm.state === 'running' ? t('hardware.liveHint') : t('hardware.mustBeStopped')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <Label>{t('hardware.media.attach.image')}</Label>
            {discs && discs.length === 0 ? (
              <p className="text-sm text-warning">{t('hardware.media.attach.noImages')}</p>
            ) : (
              <>
                <Select
                  value={imageId}
                  ariaLabel={t('hardware.media.attach.image')}
                  options={(discs ?? []).map((entry) => ({
                    value: entry.id,
                    label: `${entry.displayName} (${entry.arch})`,
                  }))}
                  onChange={setImageId}
                />
                <p className="mt-1 text-xs text-muted-foreground">{t('hardware.media.attach.hint')}</p>
              </>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={!disc || attach.isPending} onClick={submit}>
            {t('hardware.media.attach.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
