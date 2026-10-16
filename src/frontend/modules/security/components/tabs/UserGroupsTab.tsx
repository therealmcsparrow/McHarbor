// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Group memberships for one user.
//
// Lifted verbatim out of the users rail, where it shared a third of the
// page with four other lists. It now owns the width, so a group name no
// longer truncates next to its own remove button.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconUsersGroup } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useUserGroups } from '@resources/hooks/useUsers';
import { useGroups, useAddGroupMember, useRemoveGroupMember } from '../../hooks/useGroups';
import { GroupAssignmentDialog } from '../GroupAssignmentDialog';

type UserGroupsTabProps = {
  userId: string;
};

export function UserGroupsTab({ userId }: UserGroupsTabProps) {
  const { t } = useTranslation('security');
  const { data: userGroups = [], isLoading } = useUserGroups(userId);
  const { data: allGroups = [] } = useGroups();
  const addMember = useAddGroupMember();
  const removeMember = useRemoveGroupMember();
  const [assignOpen, setAssignOpen] = useState(false);

  const assignedGroupIds = new Set(userGroups.map((group) => group.groupId));
  const availableGroups = allGroups.filter((group) => !assignedGroupIds.has(group.id));

  return (
    <Card>
      <CardHeader className="flex items-center justify-between gap-3">
        <CardTitle className="text-sm">{t('users.groupAssignments')}</CardTitle>
        <Button permission={PERM.GROUPS_MANAGE} size="sm" onClick={() => setAssignOpen(true)}>
          {t('users.assignGroup')}
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Spinner />
        ) : userGroups.length === 0 ? (
          <EmptyState variant="compact" icon={<IconUsersGroup className="size-5" />} title={t('users.noGroups')} />
        ) : (
          <ul className="space-y-2">
            {userGroups.map((group) => (
              <li
                key={group.id}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium">{group.groupName}</span>
                  {group.isSystem && <Badge variant="secondary">{t('groups.system')}</Badge>}
                  <span className="text-xs text-muted-foreground">
                    {t('groups.memberCount', { count: group.memberCount })}
                  </span>
                </div>
                <Button
                  permission={PERM.GROUPS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => removeMember.mutate({ groupId: group.groupId, userId })}
                  aria-label={t('users.removeFromGroup')}
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <GroupAssignmentDialog
        open={assignOpen}
        onOpenChange={setAssignOpen}
        groups={availableGroups}
        onAssign={(groupId) => {
          addMember.mutate({ groupId, userId });
          setAssignOpen(false);
        }}
      />
    </Card>
  );
}
