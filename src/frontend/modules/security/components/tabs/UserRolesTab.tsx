// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Role assignments for one user.
//
// Assignments are grouped by role + scope before rendering, so a role
// granted across eight stacks is one row naming eight stacks rather than
// eight rows naming the same role — `groupRoleAssignments` already did
// this in the rail and keeps doing it here.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconShieldCheck, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useUserRoles, useAssignUserRole, useUnassignUserRole } from '@resources/hooks/useUsers';
import { useRoles } from '../../hooks/useRoles';
import { RoleAssignmentDialog } from '../RoleAssignmentDialog';
import { formatGroupedRoleScope, groupRoleAssignments } from '../role-assignment-utils';

type UserRolesTabProps = {
  userId: string;
};

export function UserRolesTab({ userId }: UserRolesTabProps) {
  const { t } = useTranslation('security');
  const { data: userRoles = [], isLoading } = useUserRoles(userId);
  const { data: allRoles = [] } = useRoles();
  const assignRole = useAssignUserRole();
  const unassignRole = useUnassignUserRole();
  const [assignOpen, setAssignOpen] = useState(false);

  const groupedRoles = groupRoleAssignments(userRoles);

  return (
    <Card>
      <CardHeader className="flex items-center justify-between gap-3">
        <CardTitle className="text-sm">{t('users.roleAssignments')}</CardTitle>
        <Button permission={PERM.PERMISSIONS_ASSIGN} size="sm" onClick={() => setAssignOpen(true)}>
          {t('users.assignRole')}
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Spinner />
        ) : groupedRoles.length === 0 ? (
          <EmptyState variant="compact" icon={<IconShieldCheck className="size-5" />} title={t('users.noRoles')} />
        ) : (
          <ul className="space-y-2">
            {groupedRoles.map((role) => (
              <li
                key={role.key}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <div className="min-w-0">
                  <span className="block truncate text-sm font-medium">{role.roleName}</span>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {formatGroupedRoleScope(role, t('users.global'))}
                  </span>
                </div>
                <Button
                  permission={PERM.PERMISSIONS_ASSIGN}
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => unassignRole.mutate({ userId, assignmentIds: role.assignmentIds })}
                  aria-label={t('toast.roleUnassigned')}
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <RoleAssignmentDialog
        open={assignOpen}
        onOpenChange={setAssignOpen}
        roles={allRoles}
        existingAssignments={userRoles}
        onAssign={(roleId, environmentId, stackNames) => {
          assignRole.mutate({ userId, roleId, environmentId, stackNames });
          setAssignOpen(false);
        }}
      />
    </Card>
  );
}
