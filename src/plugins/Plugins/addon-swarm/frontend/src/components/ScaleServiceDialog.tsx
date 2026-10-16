// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '../permissions';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Spinner } from '@resources/components/ui/Spinner';
import { useScaleSwarmService } from '../hooks/useSwarmServices';
import type { SwarmService } from '../types';

/** The maximum the backend accepts. Kept in step with `maxReplicas` in Go. */
const MAX_REPLICAS = 1000;

type ScaleServiceDialogProps = {
  service: SwarmService | null;
  onOpenChange: (open: boolean) => void;
};

export function ScaleServiceDialog({ service, onOpenChange }: ScaleServiceDialogProps) {
  const { t } = useTranslation('swarm');
  const scale = useScaleSwarmService();
  const [replicas, setReplicas] = useState(0);

  // Re-seed whenever a different service opens the dialog, so it never shows
  // the replica count of the service the user looked at before this one.
  const [seenService, setSeenService] = useState(service);
  if (seenService !== service) {
    setSeenService(service);
    if (service) setReplicas(service.replicas);
  }

  const valid = Number.isInteger(replicas) && replicas >= 0 && replicas <= MAX_REPLICAS;

  return (
    <Dialog open={service !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.scale.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.scale.description', { name: service?.name ?? '' })}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <label className="block space-y-1.5">
            <span className="text-sm font-medium text-foreground">{t('fields.replicas')}</span>
            <NumberInput value={replicas} onChange={(next) => setReplicas(next ?? 0)} min={0} max={MAX_REPLICAS} />
          </label>
          {/* Scaling to zero is a legitimate thing to want and a surprising
              thing to do by accident, so it is called out rather than blocked. */}
          {replicas === 0 && <p className="text-xs text-muted-foreground">{t('dialogs.scale.zeroHint')}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={PERM.SWARM_SERVICES_SCALE}
            disabled={!valid || scale.isPending}
            onClick={() => {
              if (!service) return;
              scale.mutate({ id: service.id, replicas }, { onSuccess: () => onOpenChange(false) });
            }}
          >
            {scale.isPending && <Spinner size="sm" />}
            {t('actions.scale')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
