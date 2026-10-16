// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useDryRun, useSyncNow, type Endpoint } from '@resources/hooks/useScimEndpoints';
import { SyncPlanView } from '@resources/components/security/scim/SyncPlanView';

type SyncPreviewDialogProps = {
  endpoint: Pick<Endpoint, 'id' | 'name'> | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function SyncPreviewDialog({ endpoint, onOpenChange, onSuccess }: SyncPreviewDialogProps) {
  const { t } = useTranslation('scim');
  const isOpen = endpoint !== null;
  const preview = useDryRun(endpoint?.id ?? '', isOpen);
  const syncNow = useSyncNow();
  const { reset } = syncNow;

  useEffect(() => {
    if (isOpen) reset();
  }, [isOpen, reset]);

  const result = preview.data;
  const plan = result?.plan;
  const failure = preview.error?.message ?? (result?.status === 'failed' ? result.error : undefined);

  const confirmAction = () => {
    if (!endpoint) return;
    syncNow.mutate(endpoint.id, {
      onSuccess: () => {
        onOpenChange(false);
        onSuccess?.();
      },
    });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('preview.title', { name: endpoint?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('preview.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {preview.isFetching && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner />
              {t('preview.loading')}
            </div>
          )}
          {!preview.isFetching && failure && (
            <Alert variant="destructive" title={t('preview.failed')}>
              <span className="break-words whitespace-pre-wrap">{failure}</span>
            </Alert>
          )}
          {plan?.blocked && (
            <Alert variant="warning" title={t('preview.blockedTitle')}>
              {t(`blocked.${plan.blockedReason ?? 'empty_directory'}`)}
            </Alert>
          )}
          {!preview.isFetching && plan && <SyncPlanView plan={plan} />}
          <p className="text-xs text-muted-foreground">{t('preview.queuedNote')}</p>
          {syncNow.isError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {syncNow.error.message}
            </p>
          )}
        </DialogBody>
        <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
          <Button variant="ghost" onClick={() => void preview.refetch()} disabled={preview.isFetching}>
            <IconRefresh className="size-4" />
            {t('preview.refresh')}
          </Button>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              permission={PERM.SCIM_MANAGE}
              onClick={confirmAction}
              disabled={syncNow.isPending || plan?.blocked === true}
            >
              {syncNow.isPending ? t('common:actions.processing') : t('preview.confirm')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
