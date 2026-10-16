// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconShieldCheck } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { ApiError } from '@resources/utils/api-mutation';

type WorkflowSaveRefusalProps = {
  error: Error | null;
};

/**
 * A save refused for host_self.manage stays on the page with the server's own message,
 * because the toast disappears while the operator still has to find out which key to ask for.
 */
export function WorkflowSaveRefusal({ error }: WorkflowSaveRefusalProps) {
  const { t } = useTranslation('common');
  if (!(error instanceof ApiError) || error.status !== 403) return null;
  return (
    <Alert variant="destructive" title={t('workflows.selfHostRefusedTitle')} className="m-2">
      {error.message}
    </Alert>
  );
}

type SelfHostAuthorizedBadgeProps = {
  authorized: boolean | undefined;
};

/** Read-only: the flag is set by the server from whoever last saved the workflow. */
export function SelfHostAuthorizedBadge({ authorized }: SelfHostAuthorizedBadgeProps) {
  const { t } = useTranslation('common');
  if (!authorized) return null;
  return (
    <Badge variant="info" title={t('workflows.selfHostAuthorizedHint')} className="gap-1">
      <IconShieldCheck className="size-3" aria-hidden="true" />
      {t('workflows.selfHostAuthorized')}
    </Badge>
  );
}
