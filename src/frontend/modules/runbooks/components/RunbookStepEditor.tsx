// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowDown, IconArrowUp, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { useEntryDescription } from '../hooks/useEntryDescription';
import { RunbookStepParams } from './RunbookStepParams';
import type { RunbookCatalog, StepInput, StepKind } from '@resources/types/runbooks';

type RunbookStepEditorProps = {
  index: number;
  total: number;
  step: StepInput;
  catalog: RunbookCatalog | undefined;
  envId: string;
  onChange: (next: StepInput) => void;
  onMove: (delta: number) => void;
  onRemove: () => void;
};

export function RunbookStepEditor({
  index,
  total,
  step,
  catalog,
  envId,
  onChange,
  onMove,
  onRemove,
}: RunbookStepEditorProps) {
  const { t } = useTranslation('runbooks');
  const describe = useEntryDescription();
  const entries = step.kind === 'action' ? (catalog?.actions ?? []) : (catalog?.diagnostics ?? []);
  const selectedKey = step.kind === 'action' ? step.actionKey : step.diagnosticKey;
  const selected = entries.find((entry) => entry.key === selectedKey);

  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-muted-foreground">
          {t('editor.stepNumber', { position: index + 1 })}
        </span>
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('editor.moveUp')}
            disabled={index === 0}
            onClick={() => onMove(-1)}
          >
            <IconArrowUp className="size-4" aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('editor.moveDown')}
            disabled={index === total - 1}
            onClick={() => onMove(1)}
          >
            <IconArrowDown className="size-4" aria-hidden="true" />
          </Button>
          <Button variant="ghost" size="icon" aria-label={t('editor.removeStep')} onClick={onRemove}>
            <IconTrash className="size-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <Input
        aria-label={t('editor.stepTitle')}
        placeholder={t('editor.stepTitle')}
        value={step.title}
        onChange={(eventItem) => onChange({ ...step, title: eventItem.target.value })}
      />
      <Textarea
        aria-label={t('editor.stepBody')}
        placeholder={t('editor.stepBody')}
        rows={2}
        value={step.body}
        onChange={(evt) => onChange({ ...step, body: evt.target.value })}
      />

      <div className="flex flex-wrap items-center gap-3">
        <Select
          ariaLabel={t('editor.stepKind')}
          value={step.kind}
          onChange={(value) =>
            onChange({ ...step, kind: value as StepKind, actionKey: '', diagnosticKey: '', actionParams: {} })
          }
          options={[
            { value: 'instruction', label: t('kind.instruction') },
            { value: 'diagnostic', label: t('kind.diagnostic') },
            { value: 'action', label: t('kind.action') },
          ]}
        />
        {step.kind !== 'instruction' && (
          <Select
            ariaLabel={step.kind === 'action' ? t('editor.actionKey') : t('editor.diagnosticKey')}
            value={selectedKey}
            placeholder={t('editor.pickEntry')}
            onChange={(value) =>
              onChange(
                step.kind === 'action'
                  ? { ...step, actionKey: value, actionParams: {} }
                  : { ...step, diagnosticKey: value, actionParams: {} },
              )
            }
            options={entries.map((entry) => ({
              value: entry.key,
              label: entry.plugin ? t('editor.entryFromPlugin', { key: entry.key, plugin: entry.plugin }) : entry.key,
              description: describe(entry),
            }))}
          />
        )}
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Switch checked={step.required} onCheckedChange={(checked) => onChange({ ...step, required: checked })} />
          {t('editor.required')}
        </label>
      </div>

      {selected && (
        <p className="text-xs text-muted-foreground">
          {describe(selected)} — {t('editor.needsPermission', { permission: selected.requiredPermissions.join(', ') })}
        </p>
      )}

      {selected && (
        <RunbookStepParams
          specs={selected.paramSpecs ?? []}
          targets={catalog?.targets}
          values={step.actionParams}
          envId={envId}
          onChange={(actionParams) => onChange({ ...step, actionParams })}
        />
      )}
    </li>
  );
}
