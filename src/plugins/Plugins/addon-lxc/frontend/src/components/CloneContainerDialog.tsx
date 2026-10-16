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
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { CT_PERM } from '../permissions';
import { useClone } from '../hooks/useContainers';
import type { Container } from '../types';

/** Mirrors `supportsCoW` in backend/scan.go: only these pool backends can share data. */
const SHARING_BACKENDS = ['zfs', 'btrfs', 'lvm'];

/**
 * Clone a container or a template.
 *
 * A clone is a full copy unless the pool can share data with the source, and
 * the host decides that, not this form: the switch only asks. So the outcome is
 * read back from the reply (`templatePath` is set only when the host made a
 * link) and said plainly, because "linked" also means the source cannot be
 * deleted while the clone exists.
 */
export function CloneContainerDialog({ container, onClose }: { container: Container; onClose: () => void }) {
  const { t } = useTranslation('ct');
  const clone = useClone();
  const canShare = SHARING_BACKENDS.includes(container.poolBackend);

  const [cloneName, setCloneName] = useState(`${container.name}-clone`);
  const [linked, setLinked] = useState(false);
  const [start, setStart] = useState(false);

  const result = clone.data;

  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('clone.title', { name: container.name })}</DialogTitle>
          <DialogDescription>{t('clone.body')}</DialogDescription>
        </DialogHeader>

        {result ? (
          <output className="block rounded-md bg-muted p-3 text-sm text-foreground">
            {result.templatePath
              ? t('clone.resultLinked', { name: result.name })
              : t('clone.resultFull', { name: result.name })}
          </output>
        ) : (
          <div className="space-y-4">
            <div>
              <Label htmlFor="ct-clone-name">{t('clone.name')}</Label>
              <Input
                id="ct-clone-name"
                value={cloneName}
                onChange={(ev) => setCloneName(ev.target.value)}
                autoComplete="off"
                spellCheck={false}
              />
            </div>

            <div className="flex items-start justify-between gap-3">
              <div>
                <Label htmlFor="ct-clone-linked" className="text-sm font-normal">
                  {t('clone.linked')}
                </Label>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {canShare ? t('clone.linkedHint') : t('clone.linkedUnavailable')}
                </p>
              </div>
              <Switch
                id="ct-clone-linked"
                checked={linked && canShare}
                disabled={!canShare}
                onCheckedChange={setLinked}
              />
            </div>

            <div className="flex items-start justify-between gap-3">
              <Label htmlFor="ct-clone-start" className="text-sm font-normal">
                {t('clone.start')}
              </Label>
              <Switch id="ct-clone-start" checked={start} onCheckedChange={setStart} />
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {result ? t('actions.close', { ns: 'common' }) : t('actions.cancel')}
          </Button>
          {!result && (
            <Button
              permission={CT_PERM.MANAGE}
              disabled={cloneName.trim() === '' || clone.isPending}
              onClick={() =>
                clone.mutate({ id: container.id, name: cloneName.trim(), start, linked: linked && canShare })
              }
            >
              {clone.isPending && <Spinner className="mr-2 h-4 w-4" />}
              {t('actions.clone')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
