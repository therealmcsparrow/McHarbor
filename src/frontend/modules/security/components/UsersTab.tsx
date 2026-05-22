// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  useUsers,
  useUserGroups,
  useUserRoles,
  useAssignUserRole,
  useUnassignUserRole,
} from '../hooks/useUsers';
import { useGroups, useAddGroupMember, useRemoveGroupMember } from '../hooks/useGroups';
import { GroupAssignmentDialog } from './GroupAssignmentDialog';
import { RoleAssignmentDialog } from './RoleAssignmentDialog';
import { formatGroupedRoleScope, groupRoleAssignments } from './role-assignment-utils';
import { useRoles } from '../hooks/useRoles';
import { timeAgo } from '@resources/utils/format';
import type { UserItem } from '../hooks/useUsers';

export function UsersTab() {
  const { t } = useTranslation('security');
  const { data: users, isLoading } = useUsers();
  const [selectedUser, setSelectedUser] = useState<UserItem | null>(null);

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div className="lg:col-span-2">
        <div className="rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('users.username')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('users.email')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('users.status')}</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">{t('users.lastLogin')}</th>
              </tr>
            </thead>
            <tbody>
              {users?.map((user) => (
                <tr
                  key={user.id}
                  onClick={() => setSelectedUser(user)}
                  className={`cursor-pointer border-b border-border transition-colors hover:bg-muted/30 ${
                    selectedUser?.id === user.id ? 'bg-primary/5' : ''
                  }`}
                >
                  <td className="px-4 py-2 font-medium">{user.displayName || user.username}</td>
                  <td className="px-4 py-2 text-muted-foreground">{user.email || '-'}</td>
                  <td className="px-4 py-2">
                    <Badge variant={user.isActive ? 'success' : 'secondary'}>
                      {user.isActive ? t('users.active') : t('users.inactive')}
                    </Badge>
                  </td>
                  <td className="px-4 py-2 text-muted-foreground">
                    {user.lastLogin ? timeAgo(user.lastLogin) : t('users.never')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div>
        {selectedUser ? (
          <UserGroupsPanel user={selectedUser} />
        ) : (
          <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
            {t('users.groupAssignments')}
          </div>
        )}
      </div>
    </div>
  );
}

function UserGroupsPanel({ user }: { user: UserItem }) {
  const { t } = useTranslation('security');
  const { data: userGroups, isLoading } = useUserGroups(user.id);
  const { data: userRoles, isLoading: isRolesLoading } = useUserRoles(user.id);
  const addMember = useAddGroupMember();
  const removeMember = useRemoveGroupMember();
  const assignRole = useAssignUserRole();
  const unassignRole = useUnassignUserRole();
  const [assignOpen, setAssignOpen] = useState(false);
  const [assignRoleOpen, setAssignRoleOpen] = useState(false);
  const { data: allGroups } = useGroups();
  const { data: allRoles } = useRoles();
  const groupedRoles = groupRoleAssignments(userRoles ?? []);

  const assignedGroupIds = new Set(userGroups?.map((g) => g.groupId) ?? []);
  const availableGroups = allGroups?.filter((g) => !assignedGroupIds.has(g.id)) ?? [];

  return (
    <div className="rounded-lg border border-border">
      <div className="border-b border-border px-4 py-3">
        <h3 className="text-sm font-medium">{user.displayName || user.username}</h3>
        <p className="text-xs text-muted-foreground">{t('users.groupAssignments')}</p>
      </div>

      <div className="space-y-6 p-4">
        {isLoading ? (
          <Spinner />
        ) : userGroups && userGroups.length > 0 ? (
          <ul className="space-y-2">
            {userGroups.map((group) => (
              <li key={group.id} className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                <div>
                  <span className="text-sm font-medium">{group.groupName}</span>
                  {group.isSystem && (
                    <Badge variant="secondary" className="ml-2">{t('groups.system')}</Badge>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeMember.mutate({ groupId: group.groupId, userId: user.id })}
                  aria-label={t('users.removeFromGroup')}
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">{t('users.noGroups')}</p>
        )}

        <Button variant="outline" size="sm" className="mt-3 w-full" onClick={() => setAssignOpen(true)}>
          {t('users.assignGroup')}
        </Button>

        <div className="border-t border-border pt-4">
          <div className="mb-3">
            <h4 className="text-sm font-medium">{t('users.roles')}</h4>
            <p className="text-xs text-muted-foreground">{t('users.roleAssignments')}</p>
          </div>

          {isRolesLoading ? (
            <Spinner />
          ) : groupedRoles.length > 0 ? (
            <ul className="space-y-2">
              {groupedRoles.map((role) => (
                <li key={role.key} className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                  <div>
                    <span className="text-sm font-medium">{role.roleName}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {formatGroupedRoleScope(role, t('users.global'))}
                    </span>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => unassignRole.mutate({ userId: user.id, assignmentIds: role.assignmentIds })}
                    aria-label={t('toast.roleUnassigned')}
                  >
                    <IconTrash className="size-3.5" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">{t('users.noRoles')}</p>
          )}

          <Button variant="outline" size="sm" className="mt-3 w-full" onClick={() => setAssignRoleOpen(true)}>
            {t('users.assignRole')}
          </Button>
        </div>
      </div>

      <GroupAssignmentDialog
        open={assignOpen}
        onOpenChange={setAssignOpen}
        groups={availableGroups}
        onAssign={(groupId) => {
          addMember.mutate({ groupId, userId: user.id });
          setAssignOpen(false);
        }}
      />
      <RoleAssignmentDialog
        open={assignRoleOpen}
        onOpenChange={setAssignRoleOpen}
        roles={allRoles ?? []}
        existingAssignments={userRoles ?? []}
        onAssign={(roleId, environmentId, stackNames) => {
          assignRole.mutate({ userId: user.id, roleId, environmentId, stackNames });
          setAssignRoleOpen(false);
        }}
      />
    </div>
  );
}
