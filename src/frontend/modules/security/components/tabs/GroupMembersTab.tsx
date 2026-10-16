// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The members of one group.
//
// Mirror of the user's Groups tab, seen from the other side: one card, a
// list of rows with a remove control, and an "Add member" dialog that
// offers only the users who are not already in the group.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconUsers } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { PERM } from '@core/auth/permissionKeys';
import { useUsers } from '@resources/hooks/useUsers';
import { useAddGroupMember, useRemoveGroupMember, type Group } from '../../hooks/useGroups';
import { AddGroupMemberDialog } from '../AddGroupMemberDialog';

type GroupMembersTabProps = {
  group: Group;
};

export function GroupMembersTab({ group }: GroupMembersTabProps) {
  const { t } = useTranslation('security');
  const { data: allUsers = [] } = useUsers();
  const addMember = useAddGroupMember();
  const removeMember = useRemoveGroupMember();
  const [addOpen, setAddOpen] = useState(false);

  const members = group.members ?? [];
  const memberIds = new Set(members.map((member) => member.userId));
  const availableUsers = allUsers.filter((user) => !memberIds.has(user.id));

  return (
    <Card>
      <CardHeader className="flex items-center justify-between gap-3">
        <CardTitle className="text-sm">{t('groups.members')}</CardTitle>
        <Button permission={PERM.GROUPS_MANAGE} size="sm" onClick={() => setAddOpen(true)}>
          {t('groups.addMember')}
        </Button>
      </CardHeader>
      <CardContent>
        {members.length === 0 ? (
          <EmptyState variant="compact" icon={<IconUsers className="size-5" />} title={t('groups.noMembers')} />
        ) : (
          <ul className="space-y-2">
            {members.map((member) => (
              <li
                key={member.id}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <span className="truncate text-sm font-medium">{member.username}</span>
                <Button
                  permission={PERM.GROUPS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  disabled={removeMember.isPending}
                  onClick={() => removeMember.mutate({ groupId: group.id, userId: member.userId })}
                  aria-label={t('groups.removeMember')}
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <AddGroupMemberDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        users={availableUsers}
        onAdd={(userId) => {
          addMember.mutate({ groupId: group.id, userId });
          setAddOpen(false);
        }}
      />
    </Card>
  );
}
