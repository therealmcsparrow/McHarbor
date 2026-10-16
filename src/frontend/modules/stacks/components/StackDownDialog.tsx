// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useStackAction, type StackInfo } from '@resources/hooks/useStacks';

type StackDownDialogProps = {
  stack: StackInfo | null;
  blocked?: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function StackDownDialog({ stack, blocked, onOpenChange, onSuccess }: StackDownDialogProps) {
  const { t } = useTranslation('stacks');
  const action = useStackAction();

  return (
    <MutationConfirmDialog
      open={stack !== null}
      onOpenChange={onOpenChange}
      title={t('confirm.downTitle')}
      description={t('confirm.downDescription', { name: stack?.name ?? '' })}
      confirmLabel={t('actions.down')}
      permission={PERM.STACKS_MANAGE}
      mutation={action}
      variables={{ stack: stack as StackInfo, action: 'down' }}
      blocked={blocked || stack === null}
      onSuccess={onSuccess}
    />
  );
}
