// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Pick a user to add to a group. The mirror of `GroupAssignmentDialog`,
// which picks a group to add a user to.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import type { UserItem } from '@resources/hooks/useUsers';

type AddGroupMemberDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Users not yet in the group. */
  users: UserItem[];
  onAdd: (userId: string) => void;
};

export function AddGroupMemberDialog({ open: isOpen, onOpenChange, users, onAdd }: AddGroupMemberDialogProps) {
  const { t } = useTranslation('security');
  const [userId, setUserId] = useState('');

  const userOptions = users.map((user) => ({
    value: user.id,
    label: user.displayName ? `${user.displayName} (${user.username})` : user.username,
  }));

  const handleSubmit = () => {
    if (!userId) return;
    onAdd(userId);
    setUserId('');
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('groups.addMember')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="grid gap-2">
            <Label>{t('groups.selectUser')}</Label>
            <Select
              ariaLabel={t('groups.selectUser')}
              value={userId}
              onChange={setUserId}
              options={userOptions}
              placeholder={t('groups.selectUser')}
              disabled={userOptions.length === 0}
            />
            {userOptions.length === 0 && (
              <p className="text-xs text-muted-foreground">{t('groups.allUsersAreMembers')}</p>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.GROUPS_MANAGE} onClick={handleSubmit} disabled={!userId}>
            {t('groups.addMember')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
