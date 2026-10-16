// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useRenameFile } from '../hooks/useContainerFiles';

type RenameDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  filePath: string;
  currentName: string;
};

export function RenameDialog({ open: isOpen, onOpenChange, containerId, filePath, currentName }: RenameDialogProps) {
  const { t } = useTranslation('containers');
  const [newName, setNewName] = useState(currentName);
  const renameMutation = useRenameFile(containerId);

  const [appliedName, setAppliedName] = useState(currentName);
  if (appliedName !== currentName) {
    setAppliedName(currentName);
    setNewName(currentName);
  }

  const handleSubmit = () => {
    if (!newName.trim() || newName === currentName) return;
    renameMutation.mutate({ path: filePath, newName }, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('files.rename')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="space-y-1.5">
            <Label>{t('files.newName')}</Label>
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!newName.trim() || newName === currentName || renameMutation.isPending}
          >
            {t('files.rename')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
