// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { RunbookAttachFields } from './RunbookAttachFields';
import { RunbookStepEditor } from './RunbookStepEditor';
import type { useRunbookDraft } from '../hooks/useRunbookDraft';
import type { RunbookCatalog } from '@resources/types/runbooks';

type RunbookEditorFormProps = {
  editor: ReturnType<typeof useRunbookDraft>;
  catalog: RunbookCatalog | undefined;
  fallbackEnvId: string;
};

export function RunbookEditorForm({ editor, catalog, fallbackEnvId }: RunbookEditorFormProps) {
  const { t } = useTranslation('runbooks');
  const environments = useEnvironmentList();
  const { draft, keys, patch } = editor;
  const envId = draft.environmentId || fallbackEnvId;

  return (
    <div className="flex flex-col gap-4">
      <Input
        aria-label={t('editor.name')}
        placeholder={t('editor.name')}
        value={draft.name}
        onChange={(eventItem) => patch({ name: eventItem.target.value })}
      />
      <Textarea
        aria-label={t('editor.description')}
        placeholder={t('editor.description')}
        rows={2}
        value={draft.description}
        onChange={(evt) => patch({ description: evt.target.value })}
      />

      <div className="grid gap-3 sm:grid-cols-2">
        <Select
          ariaLabel={t('editor.environment')}
          value={draft.environmentId}
          onChange={(value) => patch({ environmentId: value })}
          options={[
            { value: '', label: t('editor.allEnvironments') },
            ...(environments.data ?? []).map((env) => ({ value: env.id, label: env.name })),
          ]}
        />
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Switch checked={draft.enabled} onCheckedChange={(checked) => patch({ enabled: checked })} />
          {t('editor.enabled')}
        </label>
      </div>

      <RunbookAttachFields draft={draft} catalog={catalog} envId={envId} patch={patch} />

      <ol className="flex flex-col gap-3">
        {draft.steps.map((step, index) => (
          <RunbookStepEditor
            key={keys[index] ?? step.title}
            index={index}
            total={draft.steps.length}
            step={step}
            catalog={catalog}
            envId={envId}
            onChange={(next) => editor.updateStep(index, next)}
            onMove={(delta) => editor.moveStep(index, delta)}
            onRemove={() => editor.removeStep(index)}
          />
        ))}
      </ol>

      <Button variant="outline" permission={PERM.RUNBOOKS_MANAGE} onClick={editor.addStep}>
        <IconPlus className="size-4" aria-hidden="true" />
        {t('editor.addStep')}
      </Button>
    </div>
  );
}
