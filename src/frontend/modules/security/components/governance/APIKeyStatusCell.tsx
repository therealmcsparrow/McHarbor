// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { APIKey, APIKeyStatus } from '../../hooks/useAPIKeys';

const STATUS_VARIANT: Record<APIKeyStatus, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  active: 'success',
  expired: 'warning',
  revoked: 'destructive',
  deleted: 'secondary',
};

export function APIKeyStatusCell({ apiKey }: { apiKey: APIKey }) {
  const { t } = useTranslation('security');
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Badge variant={STATUS_VARIANT[apiKey.status]}>{t(`apiKeys.${apiKey.status}`)}</Badge>
      {apiKey.rotationDue && (
        <Badge variant="warning" title={t('governance.apiKeys.rotationDueHint', { count: apiKey.ageDays ?? 0 })}>
          {t('governance.apiKeys.rotationDue')}
        </Badge>
      )}
      {apiKey.expiringSoon && <Badge variant="warning">{t('governance.apiKeys.expiringSoon')}</Badge>}
    </div>
  );
}
