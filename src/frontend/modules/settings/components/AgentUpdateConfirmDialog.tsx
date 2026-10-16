// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useUpdateAgents } from '@resources/hooks/useAgentInventory';

export type AgentUpdateTarget = { envIds: string[]; name?: string };

type AgentUpdateConfirmDialogProps = {
  target: AgentUpdateTarget | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function AgentUpdateConfirmDialog({ target, onOpenChange, onSuccess }: AgentUpdateConfirmDialogProps) {
  const { t } = useTranslation('settings');
  const update = useUpdateAgents();
  const all = target !== null && target.envIds.length === 0;
  return (
    <MutationConfirmDialog
      open={target !== null}
      onOpenChange={onOpenChange}
      title={all ? t('about.agents.confirm.titleAll') : t('about.agents.confirm.title', { name: target?.name ?? '' })}
      description={t('about.agents.confirm.description')}
      confirmLabel={t('actions.update', { ns: 'common' })}
      variant="caution"
      permission={PERM.ENVIRONMENTS_MANAGE}
      mutation={update}
      variables={target?.envIds ?? []}
      onSuccess={onSuccess}
    />
  );
}
