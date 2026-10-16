// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The groups table's action column.
//
// Two admin actions: rename the group, delete it. Both stop the click from
// bubbling — the row itself opens the group's detail page, and pressing
// "delete" should not also navigate.
//
// Both render disabled rather than hidden on system groups, so the column
// keeps the same width on every row and the controls stay discoverable.

import { useTranslation } from 'react-i18next';
import { IconPencil, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import type { Group } from '../hooks/useGroups';

type GroupRowActionsProps = {
  group: Group;
  /** Opens the edit dialog, which the table owns. */
  onEdit: (group: Group) => void;
  /** Opens the delete confirmation, which the table owns. */
  onDelete: (group: Group) => void;
};

export function GroupRowActions({ group, onEdit, onDelete }: GroupRowActionsProps) {
  const { t } = useTranslation('security');

  return (
    <div className="flex justify-end gap-1">
      <Button
        permission={PERM.GROUPS_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.edit', { ns: 'common' })}
        disabled={group.isSystem}
        onClick={(eventItem) => {
          eventItem.stopPropagation();
          onEdit(group);
        }}
      >
        <IconPencil className="size-3.5" />
      </Button>

      <Button
        permission={PERM.GROUPS_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('groups.deleteGroup')}
        disabled={group.isSystem}
        onClick={(evt) => {
          evt.stopPropagation();
          onDelete(group);
        }}
      >
        <IconTrash className="size-3.5 text-destructive" />
      </Button>
    </div>
  );
}
