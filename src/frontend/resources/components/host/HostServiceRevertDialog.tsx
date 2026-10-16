// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useRevertHostServiceOverride } from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostServiceRevertDialogProps = {
  host: HostTarget;
  unit: string | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function HostServiceRevertDialog({ host, unit, onOpenChange, onSuccess }: HostServiceRevertDialogProps) {
  const { t } = useTranslation('environments');
  const mutation = useRevertHostServiceOverride(host);
  return (
    <MutationConfirmDialog
      open={unit !== null}
      onOpenChange={onOpenChange}
      title={t('hostServices.confirm.revert.title', { unit: unit ?? '' })}
      description={t('hostServices.confirm.revert.description', {
        unit: unit ?? '',
      })}
      confirmLabel={t('hostServices.action.revert')}
      variant="destructive"
      permission={hostPermission(host, 'servicesManage')}
      mutation={mutation}
      variables={unit ?? ''}
      onSuccess={() => onSuccess?.()}
    />
  );
}
