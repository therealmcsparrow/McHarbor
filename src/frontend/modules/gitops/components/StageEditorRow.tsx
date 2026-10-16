// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Select } from '@resources/components/ui/Select';
import type { DeployMode, StageInput } from '../types';

type StageEditorRowProps = {
  stage: StageInput;
  index: number;
  canRemove: boolean;
  environments: { id: string; name: string }[];
  onChange: (index: number, patch: Partial<StageInput>) => void;
  onRemove: (index: number) => void;
};

export function StageEditorRow({
  stage,
  index,
  canRemove,
  environments,
  onChange: handleStageChange,
  onRemove: handleRemoveStage,
}: StageEditorRowProps) {
  const { t } = useTranslation('gitops');
  return (
    <div className="rounded-md border border-border bg-card p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">
          {t('stageName')} #{index + 1}
        </span>
        {canRemove && (
          <Button variant="ghost" size="icon" onClick={() => handleRemoveStage(index)} aria-label={t('removeStage')}>
            <IconTrash className="size-4" />
          </Button>
        )}
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Input
          value={stage.stageName}
          onChange={(e) => handleStageChange(index, { stageName: e.target.value })}
          placeholder={t('placeholders.environmentName')}
        />
        <Select
          value={stage.targetEnvironmentId}
          onChange={(v) => handleStageChange(index, { targetEnvironmentId: v })}
          options={environments.map((e) => ({
            value: e.id,
            label: e.name,
          }))}
          placeholder={environments.length === 0 ? t('envRequired') : t('selectEnvironment')}
        />
        <Input
          value={stage.branch}
          onChange={(e) => handleStageChange(index, { branch: e.target.value })}
          placeholder={t('placeholders.branch')}
        />
        <Input
          value={stage.composePath}
          onChange={(e) => handleStageChange(index, { composePath: e.target.value })}
          placeholder={t('placeholders.composePath')}
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
            onCheckedChange={(v) => handleStageChange(index, { requiresApproval: v })}
          />
          {t('stageRequiresApproval')}
        </label>
      </div>
    </div>
  );
}
