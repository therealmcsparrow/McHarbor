// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useInstallDebugShell, type DebugShellSession } from '../hooks/useDebugShell';
import { useContainerHostSelfGate } from '../hooks/useHostSelfGate';

type DebugShellDialogProps = {
  containerId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: (session: DebugShellSession) => void;
};

export function DebugShellDialog({ containerId, open: isOpen, onOpenChange, onSuccess }: DebugShellDialogProps) {
  const { t } = useTranslation('containers');
  const install = useInstallDebugShell();
  const gate = useContainerHostSelfGate(containerId);

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('debugShell.title')}
      description={t('debugShell.description')}
      confirmLabel={t('debugShell.confirm')}
      variant="caution"
      permission={gate.exec(PERM.CONTAINERS_DEBUG_SHELL)}
      mutation={install}
      variables={{ id: containerId }}
      onSuccess={onSuccess}
    />
  );
}
