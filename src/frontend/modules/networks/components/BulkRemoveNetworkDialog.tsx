// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation, Trans } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { isBuiltInNetwork, isNetworkInUse } from '@core/utils/network';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { useRemoveNetwork } from '@resources/hooks/useNetworks';

type BulkRemoveNetworkDialogProps = {
  networks: NetworkInfo[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function BulkRemoveNetworkDialog({
  networks,
  open,
  onOpenChange,
}: BulkRemoveNetworkDialogProps) {
  const { t } = useTranslation('networks');
  const removeMutation = useRemoveNetwork();
  const [confirm, setConfirm] = useState(false);

  const removable = useMemo(
    () => networks.filter((n) => !isNetworkInUse(n) && !isBuiltInNetwork(n)),
    [networks],
  );
  const inUse = useMemo(() => networks.filter(isNetworkInUse), [networks]);
  const builtIn = useMemo(() => networks.filter(isBuiltInNetwork), [networks]);
  const allInUse = networks.length > 0 && removable.length === 0;

  useEffect(() => {
    if (open) setConfirm(false);
  }, [open, networks.length]);

  if (!open || networks.length === 0) return null;

  const handleRemove = async () => {
    for (const n of removable) {
      try {
        await removeMutation.mutateAsync(n.Id);
      } catch {
        // Per-network failures are surfaced via the mutation's error toast;
        // continue with the remaining networks.
      }
    }
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('bulkRemoveDialog.title')}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="bulkRemoveDialog.description"
              ns="networks"
              values={{ count: removable.length }}
              components={{ bold: <span className="font-medium text-foreground" /> }}
            />
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 px-4 py-3">
          <div className="max-h-40 overflow-y-auto rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs">
            <div className="mb-1 text-muted-foreground">{t('bulkRemoveDialog.networks')}</div>
            {removable.length === 0 ? (
              <div className="text-muted-foreground">{t('bulkRemoveDialog.noneEligible')}</div>
            ) : (
              <ul className="space-y-0.5 font-mono text-foreground">
                {removable.slice(0, 10).map((n) => (
                  <li key={n.Id} className="truncate">
                    {n.Name}
                  </li>
                ))}
                {removable.length > 10 && (
                  <li className="text-muted-foreground">
                    {t('bulkRemoveDialog.andXMore', { count: removable.length - 10 })}
                  </li>
                )}
              </ul>
            )}
          </div>

          {builtIn.length > 0 && (
            <div className="rounded-lg border border-sky-500/30 bg-sky-500/10 px-3 py-2 text-xs text-sky-700 dark:text-sky-300">
              <div className="mb-1 flex items-center gap-1.5 font-medium">
                <IconLock className="h-3.5 w-3.5" aria-hidden="true" />
                {t('bulkRemoveDialog.builtInSkipped', { count: builtIn.length })}
              </div>
              <ul className="ml-5 list-disc font-mono">
                {builtIn.slice(0, 5).map((n) => (
                  <li key={n.Id} className="truncate">
                    {n.Name}
                  </li>
                ))}
                {builtIn.length > 5 && (
                  <li className="text-muted-foreground">
                    {t('bulkRemoveDialog.andXMore', { count: builtIn.length - 5 })}
                  </li>
                )}
              </ul>
            </div>
          )}

          {inUse.length > 0 && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
              {t('bulkRemoveDialog.inUseWarning', { count: inUse.length })}
            </div>
          )}

          {allInUse && (
            <div className="flex items-center justify-between">
              <div className="text-sm">{t('bulkRemoveDialog.forceLabel')}</div>
              <Switch checked={confirm} onCheckedChange={setConfirm} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            variant="destructive"
            onClick={handleRemove}
            disabled={removeMutation.isPending || removable.length === 0 || (allInUse && !confirm)}
          >
            {removeMutation.isPending
              ? t('bulkRemoveDialog.removing')
              : t('actions.remove', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
