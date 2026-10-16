// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { releaseLabel, useDeleteAgentRelease, type AgentReleaseBuild } from '../hooks/useAgentReleases';

type AgentReleaseDeleteDialogProps = {
  release: AgentReleaseBuild | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function AgentReleaseDeleteDialog({ release, onOpenChange, onSuccess }: AgentReleaseDeleteDialogProps) {
  const { t } = useTranslation('settings');
  const remove = useDeleteAgentRelease();
  return (
    <MutationConfirmDialog
      open={release !== null}
      onOpenChange={onOpenChange}
      title={t('agentReleases.deleteTitle', { version: release ? releaseLabel(release) : '' })}
      description={t('agentReleases.deleteDescription')}
      confirmLabel={t('actions.delete', { ns: 'common' })}
      variant="destructive"
      permission={PERM.UPDATES_MANAGE}
      mutation={remove}
      variables={release ?? { version: '' }}
      onSuccess={onSuccess}
    />
  );
}
