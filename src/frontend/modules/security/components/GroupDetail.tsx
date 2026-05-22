// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Select } from '@resources/components/ui/Select';
import {
  useGroup,
  useAddGroupMember,
  useRemoveGroupMember,
  useAssignGroupRole,
  useUnassignGroupRole,
} from '../hooks/useGroups';
import { useUsers } from '../hooks/useUsers';
import { useRoles } from '../hooks/useRoles';
import { RoleAssignmentDialog } from './RoleAssignmentDialog';
import { formatGroupedRoleScope, groupRoleAssignments } from './role-assignment-utils';

type GroupDetailProps = {
  groupId: string;
  onDelete: () => void;
};

export function GroupDetail({ groupId, onDelete }: GroupDetailProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const { data: group, isLoading } = useGroup(groupId);
  const { data: allUsers } = useUsers();
  const { data: allRoles } = useRoles();
  const addMember = useAddGroupMember();
  const removeMember = useRemoveGroupMember();
  const unassignRole = useUnassignGroupRole();
  const [roleDialogOpen, setRoleDialogOpen] = useState(false);
  const [memberUserId, setMemberUserId] = useState('');
  const assignGroupRole = useAssignGroupRole();

  if (isLoading || !group) {
    return <Spinner />;
  }

  const nonMembers = allUsers?.filter((u) => !group.members?.some((m) => m.userId === u.id)) ?? [];
  const groupedRoles = groupRoleAssignments(group.roles ?? []);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">{group.name}</h3>
          {group.description && <p className="text-sm text-muted-foreground">{group.description}</p>}
        </div>
        {group.isSystem ? (
          <Badge variant="secondary">
            <IconLock className="mr-1 size-3" />
            {t('groups.system')}
          </Badge>
        ) : (
          <Button variant="destructive" size="sm" onClick={onDelete}>
            <IconTrash className="mr-1 size-3.5" />
            {tc('actions.delete')}
          </Button>
        )}
      </div>

      {/* Members */}
      <div className="rounded-lg border border-border">
        <div className="border-b border-border px-4 py-2">
          <h4 className="text-sm font-medium">{t('groups.members')}</h4>
        </div>
        <div className="p-4">
          {group.members && group.members.length > 0 ? (
            <ul className="space-y-1">
              {group.members.map((m) => (
                <li key={m.id} className="flex items-center justify-between rounded px-2 py-1 hover:bg-muted/30">
                  <span className="text-sm">{m.username}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeMember.mutate({ groupId, userId: m.userId })}
                    aria-label={tc('actions.remove')}
                  >
                    <IconTrash className="size-3.5" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">{t('groups.noMembers')}</p>
          )}

          {nonMembers.length > 0 && (
            <div className="mt-3 flex gap-2">
              <Select
                value={memberUserId}
                onChange={setMemberUserId}
                options={nonMembers.map((u) => ({ value: u.id, label: u.username }))}
                placeholder={t('groups.selectUser')}
                className="flex-1"
              />
              <Button
                variant="outline"
                size="sm"
                disabled={!memberUserId}
                onClick={() => {
                  addMember.mutate({ groupId, userId: memberUserId }, {
                    onSuccess: () => setMemberUserId(''),
                  });
                }}
              >
                {t('groups.addMember')}
              </Button>
            </div>
          )}
        </div>
      </div>

      {/* Roles */}
      <div className="rounded-lg border border-border">
        <div className="border-b border-border px-4 py-2">
          <h4 className="text-sm font-medium">{t('groups.roles')}</h4>
        </div>
        <div className="p-4">
          {groupedRoles.length > 0 ? (
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
                    onClick={() => unassignRole.mutate({ groupId, assignmentIds: role.assignmentIds })}
                    aria-label={tc('actions.remove')}
                  >
                    <IconTrash className="size-3.5" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">{t('groups.noRoles')}</p>
          )}

          <Button variant="outline" size="sm" className="mt-3" onClick={() => setRoleDialogOpen(true)}>
            {t('groups.assignRole')}
          </Button>
        </div>
      </div>

      <RoleAssignmentDialog
        open={roleDialogOpen}
        onOpenChange={setRoleDialogOpen}
        roles={allRoles ?? []}
        existingAssignments={group.roles ?? []}
        onAssign={(roleId, environmentId, stackNames) => {
          assignGroupRole.mutate({ groupId, roleId, environmentId, stackNames });
          setRoleDialogOpen(false);
        }}
      />
    </div>
  );
}

