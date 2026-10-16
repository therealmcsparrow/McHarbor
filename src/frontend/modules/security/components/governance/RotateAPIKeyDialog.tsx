// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRotateAPIKey, type APIKey, type CreateAPIKeyResult } from '../../hooks/useAPIKeys';

type RotateAPIKeyDialogProps = {
  apiKey: APIKey | null;
  onOpenChange: (open: boolean) => void;
  onRotated: (result: CreateAPIKeyResult) => void;
};

export function RotateAPIKeyDialog({ apiKey, onOpenChange, onRotated }: RotateAPIKeyDialogProps) {
  const { t } = useTranslation('security');
  const rotate = useRotateAPIKey();
  return (
    <MutationConfirmDialog
      open={apiKey !== null}
      onOpenChange={onOpenChange}
      title={t('governance.apiKeys.rotateTitle')}
      description={t('governance.apiKeys.rotateDescription', { name: apiKey?.name ?? '' })}
      confirmLabel={t('governance.apiKeys.rotate')}
      variant="caution"
      permission={PERM.API_KEYS_MANAGE}
      mutation={rotate}
      variables={apiKey?.id ?? ''}
      onSuccess={onRotated}
    />
  );
}
