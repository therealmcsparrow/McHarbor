// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { Group } from '../hooks/useGroups';

type GroupAssignmentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groups: Group[];
  onAssign: (groupId: string) => void;
};

export function GroupAssignmentDialog({ open, onOpenChange, groups, onAssign }: GroupAssignmentDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const [groupId, setGroupId] = useState('');

  const groupOptions = groups.map((g) => ({ value: g.id, label: g.name }));

  const handleSubmit = () => {
    if (!groupId) return;
    onAssign(groupId);
    setGroupId('');
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('users.assignGroup')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <label className="mb-1 block text-sm font-medium">{t('users.selectGroup')}</label>
            <Select
              value={groupId}
              onChange={setGroupId}
              options={groupOptions}
              placeholder={t('users.selectGroup')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!groupId}>
            {t('users.assignGroup')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
