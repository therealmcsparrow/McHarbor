// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
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
import { useMappingPreview, type MappingConfig } from '../../hooks/useIdentitySetup';
import { RoleImpactList } from './RoleImpactList';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  providerType: string;
  config: MappingConfig;
  saving: boolean;
  saveError?: string;
  onConfirm: () => void;
};

export function MappingImpactConfirmDialog({
  open: isOpen,
  onOpenChange,
  providerId,
  providerType,
  config,
  saving,
  saveError,
  onConfirm,
}: Props) {
  const { t } = useTranslation('access');
  const preview = useMappingPreview();
  const { mutate, reset } = preview;
  const configKey = JSON.stringify(config);

  useEffect(() => {
    if (!isOpen) {
      reset();
      return;
    }
    mutate({ providerId, providerType, config: JSON.parse(configKey) as MappingConfig, sampleGroups: [] });
  }, [isOpen, providerId, providerType, configKey, mutate, reset]);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('impact.confirmTitle')}</DialogTitle>
          <DialogDescription>{t('impact.confirmDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {preview.isPending && <Spinner />}
          {preview.isError && <Alert variant="destructive">{preview.error.message}</Alert>}
          {preview.data?.impact && <RoleImpactList impact={preview.data.impact} />}
          <p className="text-xs text-muted-foreground">{t('impact.nextSignIn')}</p>
          {saveError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {saveError}
            </p>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:actions.cancel')}
          </Button>
          <Button permission={PERM.IDENTITY_PROVIDERS_MANAGE} onClick={onConfirm} disabled={saving}>
            {saving ? t('common:actions.processing') : t('impact.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
