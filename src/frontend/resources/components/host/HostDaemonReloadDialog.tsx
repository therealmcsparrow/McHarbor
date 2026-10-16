// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useHostDaemonReload } from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostDaemonReloadDialogProps = {
  host: HostTarget;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function HostDaemonReloadDialog({ host, open: isOpen, onOpenChange, onSuccess }: HostDaemonReloadDialogProps) {
  const { t } = useTranslation('environments');
  const mutation = useHostDaemonReload(host);
  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('hostServices.confirm.reload.title')}
      description={t('hostServices.confirm.reload.description')}
      confirmLabel={t('hostServices.action.daemonReload')}
      variant="caution"
      permission={hostPermission(host, 'servicesManage')}
      mutation={mutation}
      variables={undefined}
      onSuccess={() => onSuccess?.()}
    />
  );
}
