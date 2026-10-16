// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Create a group, or rename one. The same two fields either way, so one
// dialog serves both: pass `group` to edit it, leave it out to create.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { PERM } from '@core/auth/permissionKeys';
import { useCreateGroup, useUpdateGroup, type Group } from '../hooks/useGroups';

type GroupFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The group to edit. Omit to create a new one. */
  group?: Group | null;
};

export function GroupFormDialog({ open: isOpen, onOpenChange, group }: GroupFormDialogProps) {
  const { t } = useTranslation('security');
  const createGroup = useCreateGroup();
  const updateGroup = useUpdateGroup();
  const [itemName, setName] = useState('');
  const [description, setDescription] = useState('');

  const editing = !!group;
  const pending = createGroup.isPending || updateGroup.isPending;

  // Seed the form from the group each time the dialog opens, so a stale
  // draft from a previous open never leaks into the next one.
  const [seeded, setSeeded] = useState<{ isOpen: boolean; group: typeof group }>({ isOpen: false, group: undefined });
  if (seeded.isOpen !== isOpen || seeded.group !== group) {
    setSeeded({ isOpen, group });
    if (isOpen) {
      setName(group?.name ?? '');
      setDescription(group?.description ?? '');
    }
  }

  function handleSubmit() {
    const body = { name: itemName.trim(), description: description.trim() };
    const onSuccess = () => onOpenChange(false);
    if (group) {
      updateGroup.mutate({ id: group.id, ...body }, { onSuccess });
    } else {
      createGroup.mutate(body, { onSuccess });
    }
  }

  const canSubmit = itemName.trim() !== '' && !pending;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? t('groups.editGroup') : t('groups.createGroup')}</DialogTitle>
          <DialogDescription>
            {editing ? t('groups.editGroupDescription') : t('groups.createGroupDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="group-form-name">{t('groups.name')}</Label>
            <Input
              id="group-form-name"
              value={itemName}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('groups.name')}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="group-form-description">{t('groups.groupDescription')}</Label>
            <Input
              id="group-form-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('groups.groupDescription')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.GROUPS_MANAGE} onClick={handleSubmit} disabled={!canSubmit}>
            {pending
              ? t('actions.processing', { ns: 'common' })
              : editing
                ? t('actions.save', { ns: 'common' })
                : t('actions.create', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
