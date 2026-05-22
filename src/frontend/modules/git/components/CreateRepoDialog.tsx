// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { UseMutationResult } from '@tanstack/react-query';

type CreateRepoDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createRepo: UseMutationResult<unknown, Error, { name: string; url: string; branch: string }>;
};

export function CreateRepoDialog({ open, onOpenChange, createRepo }: CreateRepoDialogProps) {
  const { t } = useTranslation('common');
  const [repoName, setRepoName] = useState('');
  const [repoUrl, setRepoUrl] = useState('');
  const [repoBranch, setRepoBranch] = useState('main');

  const handleCreate = () => {
    if (!repoName.trim() || !repoUrl.trim()) return;
    createRepo.mutate(
      { name: repoName.trim(), url: repoUrl.trim(), branch: repoBranch.trim() || 'main' },
      {
        onSuccess: () => {
          onOpenChange(false);
          setRepoName('');
          setRepoUrl('');
          setRepoBranch('main');
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('git.addTitle')}</DialogTitle>
          <DialogDescription>{t('git.addDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('git.nameLabel')}</Label>
            <Input
              type="text"
              value={repoName}
              onChange={(e) => setRepoName(e.target.value)}
              placeholder="my-project"
            />
          </div>
          <div>
            <Label className="mb-2">{t('git.repoUrlLabel')}</Label>
            <Input
              type="text"
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              placeholder="https://github.com/user/repo.git"
            />
          </div>
          <div>
            <Label className="mb-2">{t('git.branchLabel')}</Label>
            <Input
              type="text"
              value={repoBranch}
              onChange={(e) => setRepoBranch(e.target.value)}
              placeholder="main"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleCreate} disabled={createRepo.isPending || !repoName.trim() || !repoUrl.trim()}>
            {createRepo.isPending ? t('git.adding') : t('git.addRepository')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

