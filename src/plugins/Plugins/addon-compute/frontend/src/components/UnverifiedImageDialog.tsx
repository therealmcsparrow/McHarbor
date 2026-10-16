// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { COMPUTE_PERM } from '../permissions';
import type { ComputeImage } from '../types';

/**
 * The acknowledgement for downloading an image with no published checksum.
 *
 * This is deliberately a separate, deliberate step rather than a toast the
 * operator can click past. The image becomes a guest's boot disk, and there is
 * no way to tell whether what arrives is what the publisher intended — so the
 * consequence is stated in those terms, the checkbox has to be ticked, and the
 * backend writes the acknowledgement onto the audit row.
 *
 * Built-in catalog entries never reach this dialog: their verification is
 * mandatory and cannot be waived at all.
 */
export function UnverifiedImageDialog({
  image,
  onClose,
  onConfirm,
}: {
  image: ComputeImage;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation('compute');
  const [acknowledged, setAcknowledged] = useState(false);

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('images.unverified.title')}</DialogTitle>
          <DialogDescription>{image.displayName}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex items-start gap-3 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
            <p className="text-sm text-foreground">{t('images.unverified.body')}</p>
          </div>

          <div>
            <p className="text-xs text-muted-foreground">{t('images.add.sourceUrl')}</p>
            <code className="mt-1 block overflow-x-auto rounded bg-muted px-2 py-1.5 font-mono text-xs text-foreground">
              {image.sourceUrl}
            </code>
          </div>

          <div className="flex items-start gap-2">
            <Checkbox
              id="ack-unverified"
              checked={acknowledged}
              onCheckedChange={(next) => setAcknowledged(next === true)}
            />
            <Label htmlFor="ack-unverified" className="text-sm font-normal">
              {t('images.unverified.acknowledge')}
            </Label>
          </div>

          <p className="text-xs text-muted-foreground">{t('images.unverified.recorded')}</p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button variant="destructive" permission={COMPUTE_PERM.MANAGE} disabled={!acknowledged} onClick={onConfirm}>
            {t('actions.download')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
