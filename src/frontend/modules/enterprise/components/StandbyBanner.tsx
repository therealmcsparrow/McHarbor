// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { useNodeRole } from '../hooks/useCluster';

export function StandbyBanner({ className }: { className?: string }) {
  const { t } = useTranslation('enterprise');
  const role = useNodeRole();
  if (role.data?.role !== 'standby') return null;
  return (
    <Alert variant="warning" className={className} title={t('ha.banner.title')}>
      {t('ha.banner.description', { node: role.data.nodeId ?? '' })}
    </Alert>
  );
}
