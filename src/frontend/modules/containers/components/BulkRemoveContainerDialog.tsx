// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation, Trans } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { findBulkOrphanedNetworks } from '@core/utils/network';
import { useNetworks } from '@resources/hooks/useNetworks';
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
import { Switch } from '@resources/components/ui/Switch';
import { useRemoveContainer } from '../hooks/useContainers';

type BulkRemoveContainerDialogProps = {
  containers: ContainerInfo[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function BulkRemoveContainerDialog({
  containers,
  open,
  onOpenChange,
  onSuccess,
}: BulkRemoveContainerDialogProps) {
  const { t } = useTranslation('containers');
  const removeMutation = useRemoveContainer();
  const { data: networks = [] } = useNetworks();
  const [removeVolumes, setRemoveVolumes] = useState(false);
  const [removeImage, setRemoveImage] = useState(false);
  const [removeStack, setRemoveStack] = useState(false);
  const [removeNetwork, setRemoveNetwork] = useState(false);

  const containerIds = useMemo(() => containers.map((c) => c.Id), [containers]);
  const stackContainers = useMemo(
    () => containers.filter((c) => c.StackName || c.Labels?.['com.docker.compose.project']),
    [containers],
  );

  const orphanNetworks = useMemo(
    () => findBulkOrphanedNetworks(containerIds, containers, networks),
    [containerIds, containers, networks],
  );
  const hasOrphans = orphanNetworks.length > 0;

  useEffect(() => {
    if (open) {
      setRemoveVolumes(false);
      setRemoveImage(false);
      setRemoveStack(false);
      setRemoveNetwork(false);
    }
  }, [open, containerIds.join('|')]);

  if (!open || containers.length === 0) return null;

  const handleRemove = async () => {
    for (const c of containers) {
      const stackName = c.StackName ?? c.Labels?.['com.docker.compose.project'] ?? null;
      await removeMutation.mutateAsync({
        id: c.Id,
        force: true,
        removeVolumes,
        removeImage,
        removeStack: removeStack && stackName !== null,
        removeNetwork: removeNetwork && hasOrphans,
      });
    }
    onOpenChange(false);
    onSuccess?.();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('bulkRemoveDialog.title')}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="bulkRemoveDialog.description"
              ns="containers"
              values={{ count: containers.length }}
              components={{ bold: <span className="font-medium text-foreground" /> }}
            />
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 px-4 py-3">
          <div className="max-h-40 overflow-y-auto rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs">
            <div className="mb-1 text-muted-foreground">{t('bulkRemoveDialog.containers')}</div>
            <ul className="space-y-0.5 font-mono text-foreground">
              {containers.slice(0, 10).map((c) => (
                <li key={c.Id} className="truncate">
                  {c.Names?.[0]?.replace(/^\//, '') ?? c.Id}
                </li>
              ))}
              {containers.length > 10 && (
                <li className="text-muted-foreground">
                  {t('bulkRemoveDialog.andXMore', { count: containers.length - 10 })}
                </li>
              )}
            </ul>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label htmlFor="bulk-remove-volumes" className="cursor-pointer">
                <div className="text-sm font-medium">{t('removeDialog.removeVolumes')}</div>
                <div className="text-xs text-muted-foreground">{t('removeDialog.removeVolumesDesc')}</div>
              </Label>
              <Switch
                id="bulk-remove-volumes"
                checked={removeVolumes}
                onCheckedChange={setRemoveVolumes}
              />
            </div>

            <div className="flex items-center justify-between">
              <Label htmlFor="bulk-remove-image" className="cursor-pointer">
                <div className="text-sm font-medium">{t('removeDialog.removeImage')}</div>
                <div className="text-xs text-muted-foreground">{t('removeDialog.removeImageDesc')}</div>
              </Label>
              <Switch
                id="bulk-remove-image"
                checked={removeImage}
                onCheckedChange={setRemoveImage}
              />
            </div>

            {stackContainers.length > 0 && (
              <div className="flex items-center justify-between">
                <Label htmlFor="bulk-remove-stack" className="cursor-pointer">
                  <div className="text-sm font-medium">{t('removeDialog.removeStack')}</div>
                  <div className="text-xs text-muted-foreground">{t('removeDialog.removeStackDesc')}</div>
                </Label>
                <Switch
                  id="bulk-remove-stack"
                  checked={removeStack}
                  onCheckedChange={setRemoveStack}
                />
              </div>
            )}

            {hasOrphans && (
              <div className="flex items-center justify-between">
                <Label htmlFor="bulk-remove-network" className="cursor-pointer">
                  <div className="text-sm font-medium">
                    {orphanNetworks.length === 1
                      ? t('removeDialog.removeNetwork')
                      : t('removeDialog.removeNetworkCount', { count: orphanNetworks.length })}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    <span className="font-mono text-foreground">
                      {orphanNetworks.slice(0, 2).map((o) => o.name).join(', ')}
                      {orphanNetworks.length > 2 ? `, +${orphanNetworks.length - 2}` : ''}
                    </span>
                    {' — '}
                    {t('bulkRemoveDialog.removeNetworkDesc')}
                  </div>
                </Label>
                <Switch
                  id="bulk-remove-network"
                  checked={removeNetwork}
                  onCheckedChange={setRemoveNetwork}
                />
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            variant="destructive"
            onClick={handleRemove}
            disabled={removeMutation.isPending}
          >
            {removeMutation.isPending
              ? t('bulkRemoveDialog.removing')
              : t('actions.remove')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
