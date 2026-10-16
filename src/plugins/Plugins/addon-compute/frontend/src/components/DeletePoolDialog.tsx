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
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { COMPUTE_PERM } from '../permissions';
import { useDeletePool } from '../hooks/useCompute';
import type { Pool } from '../types';

/**
 * Delete a storage pool.
 *
 * Two separate decisions, presented as two separate controls:
 *
 *   1. Detach the pool from McHarbor. Nothing on the host is touched, and the
 *      agent refuses outright while guest volumes remain — naming them, so the
 *      operator learns what is in the way rather than only that something is.
 *   2. Force, which destroys those guest volumes. That is permanent data loss,
 *      so it requires both a switch and the pool's name typed out — the same
 *      pattern the stack delete uses, for the same reason.
 */

export function DeletePoolDialog({ pool, onClose }: { pool: Pool; onClose: () => void }) {
  const { t } = useTranslation('compute');
  const remove = useDeletePool();

  const [force, setForce] = useState(false);
  const [typedName, setTypedName] = useState('');

  const hasVolumes = pool.volumeCount > 0;
  // Typing the name is required only for the destructive path. Making it a
  // ceremony for the harmless one trains people to type it without reading.
  const confirmed = !force || typedName.trim() === pool.name;
  const canSubmit = confirmed && !remove.isPending && (!hasVolumes || force);

  const submit = () => {
    if (!canSubmit) return;
    remove.mutate({ poolId: pool.id, force }, { onSuccess: onClose });
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('pools.delete.title', { name: pool.name })}</DialogTitle>
          <DialogDescription>{t('pools.delete.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {hasVolumes && (
            <div className="flex items-start gap-3 rounded-md bg-warning/10 p-3">
              <IconAlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
              <p className="text-sm text-foreground">{t('pools.delete.inUse', { volumes: pool.volumeCount })}</p>
            </div>
          )}

          <div className="flex items-start justify-between gap-3">
            <Label htmlFor="pool-force" className="text-sm font-normal">
              {t('pools.delete.force')}
            </Label>
            <Switch
              id="pool-force"
              checked={force}
              onCheckedChange={(next) => {
                setForce(next);
                // Clearing the typed name on toggle-off means a second
                // toggle-on cannot inherit a confirmation from the first.
                if (!next) setTypedName('');
              }}
            />
          </div>

          {force && (
            <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <p className="text-sm font-medium text-foreground">{t('pools.delete.forceWarning')}</p>
              <div>
                <Label htmlFor="pool-confirm">{t('pools.delete.confirmName', { name: pool.name })}</Label>
                <Input
                  id="pool-confirm"
                  value={typedName}
                  onChange={(ev) => setTypedName(ev.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button variant="destructive" permission={COMPUTE_PERM.DELETE} onClick={submit} disabled={!canSubmit}>
            {remove.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('pools.delete.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
