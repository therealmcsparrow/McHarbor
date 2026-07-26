// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Select } from '@resources/components/ui/Select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import {
  useCreatePipeline,
  useUpdatePipeline,
} from '../hooks/useGitOps';
import type {
  CreatePipelineInput,
  GitOpsPipeline,
  StageInput,
  TriggerType,
  DeployMode,
} from '../types';

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
  open,
  onOpenChange,
  pipeline,
  repos,
  environments,
}: PipelineFormDialogProps) {
  const { t } = useTranslation('gitops');
  const create = useCreatePipeline(t);
  const update = useUpdatePipeline(t);

  const [name, setName] = useState(pipeline?.name ?? '');
  const [repoId, setRepoId] = useState(pipeline?.repoId ?? repos[0]?.id ?? '');
  const [description, setDescription] = useState(pipeline?.description ?? '');
  const [triggerType, setTriggerType] = useState<TriggerType>(
    pipeline?.triggerType ?? 'auto',
  );
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

  const handleAddStage = () => {
    setStages((prev) => [...prev, defaultStage(prev.length)]);
  };

  const handleRemoveStage = (index: number) => {
    setStages((prev) =>
      prev
        .filter((_, i) => i !== index)
        .map((s, i) => ({ ...s, stageIndex: i })),
    );
  };

  const handleStageChange = (index: number, patch: Partial<StageInput>) => {
    setStages((prev) =>
      prev.map((s, i) => (i === index ? { ...s, ...patch } : s)),
    );
  };

  const handleSubmit = () => {
    const input: CreatePipelineInput = {
      name,
      repoId,
      description,
      triggerType,
      stages,
    };
    if (pipeline) {
      update.mutate(
        { id: pipeline.id, input },
        { onSuccess: () => onOpenChange(false) },
      );
    } else {
      create.mutate(input, { onSuccess: () => onOpenChange(false) });
    }
  };

  const isPending = create.isPending || update.isPending;
  const isValid = name.length > 0 && repoId.length > 0 && stages.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('formTitle')}</DialogTitle>
          <DialogDescription>{t('formDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('formName')}
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="website-pipeline"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('formRepo')}
            </label>
            <Select
              value={repoId}
              onChange={setRepoId}
              options={repos.map((r) => ({ value: r.id, label: r.name }))}
              placeholder="Select repository"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('formDescriptionLabel')}
            </label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              {t('formTriggerType')}
            </label>
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
              <label className="text-xs font-medium text-muted-foreground">
                {t('formStages')}
              </label>
              <Button variant="outline" size="sm" onClick={handleAddStage}>
                <IconPlus className="mr-1.5 size-3.5" />
                {t('addStage')}
              </Button>
            </div>
            <div className="space-y-2">
              {stages.map((stage, index) => (
                <div
                  key={index}
                  className="rounded-md border border-border bg-card p-3"
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-xs font-medium text-muted-foreground">
                      {t('stageName')} #{index + 1}
                    </span>
                    {stages.length > 1 && (
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleRemoveStage(index)}
                        aria-label={t('removeStage')}
                      >
                        <IconTrash className="size-4" />
                      </Button>
                    )}
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    <Input
                      value={stage.stageName}
                      onChange={(e) =>
                        handleStageChange(index, { stageName: e.target.value })
                      }
                      placeholder="dev"
                    />
                    <Select
                      value={stage.targetEnvironmentId}
                      onChange={(v) =>
                        handleStageChange(index, { targetEnvironmentId: v })
                      }
                      options={environments.map((e) => ({
                        value: e.id,
                        label: e.name,
                      }))}
                      placeholder={
                        environments.length === 0
                          ? t('envRequired')
                          : 'Select environment'
                      }
                    />
                    <Input
                      value={stage.branch}
                      onChange={(e) =>
                        handleStageChange(index, { branch: e.target.value })
                      }
                      placeholder="main"
                    />
                    <Input
                      value={stage.composePath}
                      onChange={(e) =>
                        handleStageChange(index, { composePath: e.target.value })
                      }
                      placeholder="docker-compose.yml"
                    />
                    <Select
                      value={stage.deployMode}
                      onChange={(v) =>
                        handleStageChange(index, {
                          deployMode: v as DeployMode,
                        })
                      }
                      options={[
                        { value: 'auto', label: t('deployModeAuto') },
                        { value: 'manual', label: t('deployModeManual') },
                        { value: 'pr_preview', label: t('deployModePRPreview') },
                      ]}
                    />
                    <label className="flex items-center gap-2 text-sm">
                      <Switch
                        checked={stage.requiresApproval}
                        onCheckedChange={(v) =>
                          handleStageChange(index, { requiresApproval: v })
                        }
                      />
                      {t('stageRequiresApproval')}
                    </label>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={isPending}
          >
            {t('formActions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!isValid || isPending}>
            {pipeline ? t('formActions.update') : t('formActions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
