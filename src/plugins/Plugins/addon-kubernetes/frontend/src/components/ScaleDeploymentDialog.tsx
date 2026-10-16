// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import type { ResourceRef } from '../hooks/useKubernetesActions';
import { KUBE_PERM } from '../permissions';

type ScaleDeploymentDialogProps = {
  target: ResourceRef | null;
  /** Replica count the deployment currently reports, used to seed the field. */
  currentReplicas: number;
  onOpenChange: (open: boolean) => void;
  onConfirm: (replicas: number) => void;
  loading?: boolean;
};

// Matches maxReplicas in modules/kubernetes/model.go — the server rejects
// anything larger, so the field should not let the reader type it.
const MAX_REPLICAS = 1000;

export function ScaleDeploymentDialog({
  target,
  currentReplicas,
  onOpenChange,
  onConfirm,
  loading = false,
}: ScaleDeploymentDialogProps) {
  const { t } = useTranslation('kubernetes');
  const [replicas, setReplicas] = useState(currentReplicas);

  // Reseed whenever a different deployment opens the dialog; without this the
  // field keeps the previous target's count.
  const [seen, setSeen] = useState({ target, currentReplicas });
  if (seen.target !== target || seen.currentReplicas !== currentReplicas) {
    setSeen({ target, currentReplicas });
    if (target) setReplicas(currentReplicas);
  }

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('scale.title')}</DialogTitle>
          <DialogDescription>
            {t('scale.description', { name: target?.name ?? '', namespace: target?.namespace ?? '' })}
          </DialogDescription>
        </DialogHeader>
        <div>
          <Label className="mb-2" htmlFor="k8s-replicas">
            {t('scale.replicas')}
          </Label>
          <NumberInput id="k8s-replicas" value={replicas} onChange={setReplicas} min={0} max={MAX_REPLICAS} />
          {replicas === 0 && <p className="mt-2 text-xs text-warning">{t('scale.zeroWarning')}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={KUBE_PERM.MANAGE} onClick={() => onConfirm(replicas)} disabled={loading}>
            {loading ? t('scale.applying') : t('scale.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
