// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
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
import { StageEditorRow } from './StageEditorRow';
import { useCreatePipeline, useUpdatePipeline } from '../hooks/useGitOps';
import type { CreatePipelineInput, GitOpsPipeline, StageInput, TriggerType } from '../types';
import { PERM } from '@core/auth/permissionKeys';

type GitRepo = { id: string; name: string };
type Environment = { id: string; name: string };

type PipelineFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pipeline: GitOpsPipeline | null;
  repos: GitRepo[];
  environments: Environment[];
};

const defaultStage = (index: number): StageInput => ({
  stageName: `stage-${index}`,
  stageIndex: index,
  targetEnvironmentId: '',
  deployMode: 'auto',
  branch: 'main',
  composePath: '',
  requiresApproval: false,
});

export function PipelineFormDialog({
  open: isOpen,
  onOpenChange,
  pipeline,
  repos,
  environments,
}: PipelineFormDialogProps) {
  const { t } = useTranslation('gitops');
  const create = useCreatePipeline(t);
  const update = useUpdatePipeline(t);

  const [itemName, setName] = useState(pipeline?.name ?? '');
  const [repoId, setRepoId] = useState(pipeline?.repoId ?? repos[0]?.id ?? '');
  const [description, setDescription] = useState(pipeline?.description ?? '');
  const [triggerType, setTriggerType] = useState<TriggerType>(pipeline?.triggerType ?? 'auto');
  const [stages, setStages] = useState<StageInput[]>(
    pipeline?.stages.map((s) => ({
      stageName: s.stageName,
      stageIndex: s.stageIndex,
      targetEnvironmentId: s.targetEnvironmentId,
      deployMode: s.deployMode,
      branch: s.branch,
      composePath: s.composePath,
      requiresApproval: s.requiresApproval,
    })) ?? [defaultStage(0)],
  );
  const stageKeys = useStableListKeys(stages, (s) => s.stageName);

  const handleAddStage = () => {
    setStages((prev) => [...prev, defaultStage(prev.length)]);
  };

  const handleRemoveStage = (index: number) => {
    setStages((prev) => prev.filter((_, i) => i !== index).map((s, i) => ({ ...s, stageIndex: i })));
  };

  const handleStageChange = (index: number, patch: Partial<StageInput>) => {
    setStages((prev) => prev.map((s, i) => (i === index ? { ...s, ...patch } : s)));
  };

  const handleSubmit = () => {
    const input: CreatePipelineInput = {
      name: itemName,
      repoId,
      description,
      triggerType,
      stages,
    };
    if (pipeline) {
      update.mutate({ id: pipeline.id, input }, { onSuccess: () => onOpenChange(false) });
    } else {
      create.mutate(input, { onSuccess: () => onOpenChange(false) });
    }
  };

  const isPending = create.isPending || update.isPending;
  const isValid = itemName.length > 0 && repoId.length > 0 && stages.length > 0;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('formTitle')}</DialogTitle>
          <DialogDescription>{t('formDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('formName')}</label>
            <Input
              value={itemName}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('placeholders.pipelineName')}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('formRepo')}</label>
            <Select
              value={repoId}
              onChange={setRepoId}
              options={repos.map((r) => ({ value: r.id, label: r.name }))}
              placeholder={t('placeholders.selectRepository')}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('formDescriptionLabel')}
            </label>
            <Input value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">{t('formTriggerType')}</label>
            <Select
              value={triggerType}
              onChange={(v) => setTriggerType(v as TriggerType)}
              options={[
                { value: 'auto', label: t('triggerAuto') },
                { value: 'manual', label: t('triggerManual') },
                { value: 'pr_preview', label: t('triggerPRPreview') },
              ]}
            />
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <label className="text-xs font-medium text-muted-foreground">{t('formStages')}</label>
              <Button variant="outline" size="sm" onClick={handleAddStage}>
                <IconPlus className="mr-1.5 size-3.5" />
                {t('addStage')}
              </Button>
            </div>
            <div className="space-y-2">
              {stages.map((stage, index) => (
                <StageEditorRow
                  key={stageKeys[index]}
                  stage={stage}
                  index={index}
                  canRemove={stages.length > 1}
                  environments={environments}
                  onChange={handleStageChange}
                  onRemove={handleRemoveStage}
                />
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={isPending}>
            {t('formActions.cancel')}
          </Button>
          <Button permission={PERM.GITOPS_MANAGE} onClick={handleSubmit} disabled={!isValid || isPending}>
            {pipeline ? t('formActions.update') : t('formActions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
