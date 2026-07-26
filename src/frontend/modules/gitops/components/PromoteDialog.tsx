// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
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

type PromoteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pipeline: GitOpsPipeline;
  initialStageId?: string;
  initialCommitSha?: string;
};

export function PromoteDialog({
  open,
  onOpenChange,
  pipeline,
  initialStageId,
  initialCommitSha,
}: PromoteDialogProps) {
  const { t } = useTranslation('gitops');
  const promote = usePromoteStage(t);

  const [stageId, setStageId] = useState(
    initialStageId ?? pipeline.stages[0]?.id ?? '',
  );
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t('promoteTitle')}</DialogTitle>
          <DialogDescription>{pipeline.name}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('stageName')}
            </label>
            <select
              value={stageId}
              onChange={(e) => setStageId(e.target.value)}
              className="w-full rounded-md border border-border bg-card px-3 py-2 text-sm"
            >
              {pipeline.stages.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.stageName} (#{s.stageIndex})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('promoteCommit')}
            </label>
            <Input
              value={commitSha}
              onChange={(e) => setCommitSha(e.target.value)}
              placeholder="abc1234 or main"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('promoteNote')}
            </label>
            <Input
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={promote.isPending}
          >
            {t('formActions.cancel')}
          </Button>
          <Button
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
