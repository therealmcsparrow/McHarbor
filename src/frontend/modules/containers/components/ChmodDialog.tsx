// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useChmod } from '../hooks/useContainerFiles';

type ChmodDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  containerId: string;
  filePath: string;
  currentMode: string;
};

function extractOctal(mode: string): string {
  // Try to extract the octal from a full mode string like "drwxr-xr-x"
  if (/^[0-7]{3,4}$/.test(mode)) return mode;
  // Parse rwx format
  const perms = mode.replace(/^[dlscbp-]/, '');
  if (perms.length < 9) return '644';
  let octal = '';
  for (let i = 0; i < 3; i++) {
    const chunk = perms.slice(i * 3, i * 3 + 3);
    let val = 0;
    if (chunk[0] === 'r') val += 4;
    if (chunk[1] === 'w') val += 2;
    if (chunk[2] !== '-' && chunk[2] !== undefined) val += 1;
    octal += val;
  }
  return octal || '644';
}

export function ChmodDialog({
  open,
  onOpenChange,
  containerId,
  filePath,
  currentMode,
}: ChmodDialogProps) {
  const { t } = useTranslation('containers');
  const [mode, setMode] = useState('644');
  const chmodMutation = useChmod(containerId);

  useEffect(() => {
    setMode(extractOctal(currentMode));
  }, [currentMode]);

  const isValid = /^[0-7]{3,4}$/.test(mode);

  const handleSubmit = () => {
    if (!isValid) return;
    chmodMutation.mutate(
      { path: filePath, mode },
      { onSuccess: () => onOpenChange(false) }
    );
  };

  const fileName = filePath.split('/').pop() ?? filePath;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('files.changePermissions')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 p-4">
          <div className="text-sm text-muted-foreground font-mono">{fileName}</div>
          <div className="space-y-1.5">
            <Label>{t('files.permissions')}</Label>
            <Input
              value={mode}
              onChange={(e) => setMode(e.target.value)}
              placeholder="755"
              maxLength={4}
              className="font-mono"
              onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
              autoFocus
            />
            {!isValid && mode.length > 0 && (
              <p className="text-xs text-destructive">
                {t('files.invalidMode')}
              </p>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('edit.cancelChanges')}
          </Button>
          <Button onClick={handleSubmit} disabled={!isValid || chmodMutation.isPending}>
            {t('files.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
