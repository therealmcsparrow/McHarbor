// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteRole } from '../hooks/useRoles';

type DeleteRoleDialogProps = {
  roleId: string | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function DeleteRoleDialog({ roleId, onOpenChange, onSuccess }: DeleteRoleDialogProps) {
  const { t } = useTranslation('security');
  const deleteRole = useDeleteRole();

  return (
    <MutationConfirmDialog
      open={roleId !== null}
      onOpenChange={onOpenChange}
      title={t('roles.deleteRole')}
      description={t('roles.deleteRoleConfirm')}
      permission={PERM.ROLES_MANAGE}
      mutation={deleteRole}
      variables={roleId ?? ''}
      onSuccess={onSuccess}
    />
  );
}
