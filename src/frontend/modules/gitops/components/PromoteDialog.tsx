// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { usePromoteStage } from '../hooks/useGitOps';
import type { GitOpsPipeline } from '../types';
import { PERM } from '@core/auth/permissionKeys';

type PromoteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pipeline: GitOpsPipeline;
  initialStageId?: string;
  initialCommitSha?: string;
};

export function PromoteDialog({
  open: isOpen,
  onOpenChange,
  pipeline,
  initialStageId,
  initialCommitSha,
}: PromoteDialogProps) {
  const { t } = useTranslation('gitops');
  const promote = usePromoteStage(t);

  const [stageId, setStageId] = useState(initialStageId ?? pipeline.stages[0]?.id ?? '');
  const [commitSha, setCommitSha] = useState(initialCommitSha ?? '');
  const [note, setNote] = useState('');

  const handleSubmit = () => {
    promote.mutate(
      {
        pipelineId: pipeline.id,
        input: { stageId, commitSha, note, triggeredBy: '', prNumber: '' },
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('promoteTitle')}</DialogTitle>
          <DialogDescription>{pipeline.name}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('stageName')}</label>
            <Select
              value={stageId}
              onChange={setStageId}
              options={pipeline.stages.map((s) => ({
                value: s.id,
                label: `${s.stageName} (#${s.stageIndex})`,
              }))}
              ariaLabel={t('stageName')}
              variant="outline"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('promoteCommit')}</label>
            <Input
              value={commitSha}
              onChange={(e) => setCommitSha(e.target.value)}
              placeholder={t('placeholders.commitOrBranch')}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('promoteNote')}</label>
            <Input value={note} onChange={(e) => setNote(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={promote.isPending}>
            {t('formActions.cancel')}
          </Button>
          <Button
            permission={PERM.GITOPS_MANAGE}
            onClick={handleSubmit}
            disabled={!stageId || !commitSha || promote.isPending}
          >
            {t('promoteAction')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
