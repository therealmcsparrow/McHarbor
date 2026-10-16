// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Switch } from '@resources/components/ui/Switch';
import { useSwarmLeave } from '../hooks/useSwarmCluster';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import type { DialogProps } from './ClusterField';

/**
 * Leave the Swarm.
 *
 * On the last manager this destroys the cluster, so the confirmation says so
 * and the force switch is a separate, deliberate act rather than a default.
 */
export function LeaveClusterDialog({ open: isOpen, onOpenChange }: DialogProps) {
  const { t } = useTranslation('swarm');
  const leave = useSwarmLeave();
  const { capability } = useSwarmCapability();
  const [force, setForce] = useState(false);

  const isLastManager = capability.controlAvailable && capability.managers <= 1;

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('dialogs.leave.title')}
      description={
        <span className="block space-y-3">
          <span className="block">{t('dialogs.leave.description')}</span>
          {isLastManager && (
            <span className="flex items-start gap-2 rounded-lg border border-border bg-muted/50 p-3 text-sm text-foreground">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{t('dialogs.leave.lastManagerWarning')}</span>
            </span>
          )}
          <label className="flex items-center gap-3">
            <Switch checked={force} onCheckedChange={setForce} />
            <span className="text-sm text-foreground">{t('fields.force')}</span>
          </label>
        </span>
      }
      confirmLabel={t('actions.leaveCluster')}
      loading={leave.isPending}
      disabled={isLastManager && !force}
      onConfirm={() =>
        leave.mutate(force, {
          onSuccess: () => {
            setForce(false);
            onOpenChange(false);
          },
        })
      }
    />
  );
}
